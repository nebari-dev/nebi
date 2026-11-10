package remote

// Owner: U5 (remote). Push, sync markers, update available, server-only
// versions, server version files (used by compare, U4).
//
// Version numbers are shared with the linked server project. A local
// version is "on the server" when the server has the same number with the
// same content hash (contenthash.Hash of pixi.toml + pixi.lock). Local
// versions that never reached the server and collide with server numbers
// (two machines created version N independently) are renumbered right
// after the server's newest version, together with their tags, jobs, local
// publications and installed version. Push does it before uploading; the
// version list decorator does it as soon as it sees the collision (unless a
// job is running on the project), so the rail never shows two versions
// with the same number.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// maxPushAttempts bounds the retries when another machine pushes to the
// same server project at the same time.
const maxPushAttempts = 3

// Push uploads local versions the linked server does not have, creating
// the server project on first push (synchronous).
func (s *Service) Push(ctx context.Context, projectID uuid.UUID) (*shared.PushResult, error) {
	return s.PushUpTo(ctx, projectID, 0)
}

// PushUpTo is Push limited to local versions numbered up to upTo (0 means
// all). It is what "Push to server" on version N does: every version not
// yet on the server, oldest first, up to N. Pushing again is a no-op.
func (s *Service) PushUpTo(ctx context.Context, projectID uuid.UUID, upTo int) (*shared.PushResult, error) {
	conn, err := s.connection(ctx)
	if err != nil {
		return nil, err
	}
	db := s.deps.DB.WithContext(ctx)
	var project models.Project
	if err := db.Where("id = ?", projectID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("project %s: %w", projectID, shared.ErrNotFound)
		}
		return nil, err
	}
	locals, err := localVersions(db, projectID)
	if err != nil {
		return nil, err
	}
	if len(locals) == 0 {
		return nil, &shared.ValidationError{Message: "This project has no versions to push yet."}
	}
	if upTo > 0 {
		found := false
		for _, v := range locals {
			found = found || v.row.VersionNumber == upTo
		}
		if !found {
			return nil, fmt.Errorf("version %d: %w", upTo, shared.ErrNotFound)
		}
	}

	remote, err := s.serverRemote(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if remote != nil && !sameServer(remote.ServerURL, conn.URL) {
		return nil, &shared.ConflictError{
			Code:    "other_server",
			Message: fmt.Sprintf("This project is linked to %s. Connect to that server to push it.", displayURL(remote.ServerURL)),
		}
	}
	if remote == nil {
		if remote, err = s.linkNewServerProject(ctx, &project, conn); err != nil {
			return nil, err
		}
	}
	serverID := remote.ServerProjectID
	defer forgetServerProject(conn.URL, serverID)

	result := &shared.PushResult{PushedVersions: []int{}, ServerProjectID: serverID}
	for attempt := 1; ; attempt++ {
		st, err := s.fetchServerState(ctx, conn.URL, serverID, false)
		if err != nil {
			return nil, err
		}
		plan, err := s.reconcile(ctx, projectID, st)
		if err != nil {
			return nil, err
		}
		if n, ok := plan[upTo]; ok {
			upTo = n
		}
		locals, err := localVersions(db, projectID)
		if err != nil {
			return nil, err
		}
		var batch []localVersion
		for _, v := range locals {
			if st.onServer(v) || (upTo > 0 && v.row.VersionNumber > upTo) {
				continue
			}
			batch = append(batch, v)
		}
		result.ServerLatestVersion = st.max
		if len(batch) == 0 {
			break
		}
		req, err := s.pushRequest(db, projectID, batch, conn)
		if err != nil {
			return nil, err
		}
		var res pushVersionsResult
		err = s.ServerJSON(ctx, http.MethodPost, "/projects/"+url.PathEscape(serverID)+"/sync/versions", req, &res)
		var rerr *shared.RemoteError
		if errors.As(err, &rerr) && rerr.Status == http.StatusConflict && attempt < maxPushAttempts {
			continue // someone pushed meanwhile: look again
		}
		if errors.As(err, &rerr) && rerr.Status == http.StatusForbidden {
			return nil, &shared.RemoteError{
				Status:  http.StatusForbidden,
				Message: "You can view this project on the server but not push to it. Ask its owner for edit access.",
			}
		}
		if err != nil {
			return nil, err
		}
		for _, v := range batch {
			result.PushedVersions = append(result.PushedVersions, v.row.VersionNumber)
		}
		result.ServerLatestVersion = res.LatestVersion
		break
	}

	now := time.Now().UTC()
	if err := s.markSynced(ctx, projectID, serverID, conn.URL, now); err != nil {
		return nil, err
	}
	if err := db.Model(&models.ProjectRemote{}).Where("id = ?", remote.ID).Update("last_synced_at", now).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// linkNewServerProject creates the server project for a first push and
// records the server remote.
func (s *Service) linkNewServerProject(ctx context.Context, project *models.Project, conn *local.ServerCredentials) (*models.ProjectRemote, error) {
	var created catalogProject
	if err := s.ServerJSON(ctx, http.MethodPost, "/catalog/projects", map[string]string{"name": project.Name}, &created); err != nil {
		return nil, err
	}
	remote := models.ProjectRemote{
		ProjectID:         project.ID,
		Kind:              models.RemoteKindServer,
		Name:              conn.Name,
		ServerURL:         conn.URL,
		ServerProjectID:   created.ID,
		ServerProjectName: created.Name,
	}
	if remote.Name == "" {
		remote.Name = ServerName(conn.URL)
	}
	if err := s.deps.DB.WithContext(ctx).Create(&remote).Error; err != nil {
		return nil, fmt.Errorf("linking server project: %w", err)
	}
	return &remote, nil
}

// pushRequest builds the push body: files, description, author, tags
// (without "latest" and content-hash tags) and local publications.
func (s *Service) pushRequest(db *gorm.DB, projectID uuid.UUID, batch []localVersion, conn *local.ServerCredentials) (*pushVersionsRequest, error) {
	numbers := make([]int, len(batch))
	for i, v := range batch {
		numbers[i] = v.row.VersionNumber
	}
	var tags []models.ProjectTag
	if err := db.Where("project_id = ? AND version_number IN ?", projectID, numbers).Find(&tags).Error; err != nil {
		return nil, err
	}
	tagsBy := map[int][]string{}
	for _, t := range tags {
		if t.Tag == local.LatestTag || local.IsContentHashTag(t.Tag) {
			continue
		}
		tagsBy[t.VersionNumber] = append(tagsBy[t.VersionNumber], t.Tag)
	}
	var pubs []models.Publication
	if err := db.Preload("Registry").
		Where("project_id = ? AND version_number IN ? AND deleted_at IS NULL", projectID, numbers).
		Find(&pubs).Error; err != nil {
		return nil, err
	}
	pubsBy := map[int][]publicationRecord{}
	for _, p := range pubs {
		host := p.RegistryHost
		if host == "" {
			host = registryHost(p.Registry.URL)
		}
		if host == "" || p.Repository == "" || p.Tag == "" {
			continue
		}
		at := p.CreatedAt.UTC()
		pubsBy[p.VersionNumber] = append(pubsBy[p.VersionNumber], publicationRecord{
			RegistryHost: host,
			Repository:   p.Repository,
			Tag:          p.Tag,
			Digest:       p.Digest,
			PublishedAt:  &at,
		})
	}
	req := &pushVersionsRequest{}
	for _, v := range batch {
		author := v.row.AuthorName
		if author == "" && conn.Username != AnonymousUser {
			author = conn.Username
		}
		vt := tagsBy[v.row.VersionNumber]
		sort.Strings(vt)
		if vt == nil {
			vt = []string{}
		}
		vp := pubsBy[v.row.VersionNumber]
		if vp == nil {
			vp = []publicationRecord{}
		}
		req.Versions = append(req.Versions, pushVersion{
			Number:       v.row.VersionNumber,
			BaseVersion:  v.row.BaseVersion,
			Description:  v.row.Description,
			Author:       author,
			PixiToml:     v.row.ManifestContent,
			PixiLock:     v.row.LockFileContent,
			Tags:         vt,
			Publications: vp,
		})
	}
	return req, nil
}

// registryHost extracts the host of a registry URL ("https://ghcr.io" or
// "ghcr.io/org" -> "ghcr.io").
func registryHost(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host
	}
	u, err = url.Parse("https://" + raw)
	if err == nil {
		return u.Host
	}
	return ""
}

// markSynced records SyncedAt on every local version that the server has
// with the same content.
func (s *Service) markSynced(ctx context.Context, projectID uuid.UUID, serverID, base string, now time.Time) error {
	forgetServerProject(base, serverID)
	st, err := s.fetchServerState(ctx, base, serverID, false)
	if err != nil {
		return nil // the push itself succeeded; markers refresh later
	}
	db := s.deps.DB.WithContext(ctx)
	locals, err := localVersions(db, projectID)
	if err != nil {
		return err
	}
	var synced []int
	for _, v := range locals {
		if st.onServer(v) && v.row.SyncedAt == nil {
			synced = append(synced, v.row.VersionNumber)
		}
	}
	if len(synced) == 0 {
		return nil
	}
	return db.Model(&models.ProjectVersion{}).
		Where("project_id = ? AND version_number IN ?", projectID, synced).
		Update("synced_at", now).Error
}

// linkedServer returns the project's server remote when it points at the
// connected server, or an error suitable for the API.
func (s *Service) linkedServer(ctx context.Context, projectID uuid.UUID) (*models.ProjectRemote, *local.ServerCredentials, error) {
	conn, err := s.connection(ctx)
	if err != nil {
		return nil, nil, err
	}
	remote, err := s.serverRemote(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	if remote == nil {
		return nil, nil, &shared.ConflictError{Code: "not_linked", Message: "This project isn’t on the server yet."}
	}
	if !sameServer(remote.ServerURL, conn.URL) {
		return nil, nil, &shared.ConflictError{
			Code:    "other_server",
			Message: fmt.Sprintf("This project is linked to %s. Connect to that server to sync it.", displayURL(remote.ServerURL)),
		}
	}
	return remote, conn, nil
}

// ServerVersionFiles returns pixi.toml and pixi.lock of a version on the
// linked server (for compare against a server-only version).
func (s *Service) ServerVersionFiles(ctx context.Context, projectID uuid.UUID, number int) (pixiToml, pixiLock string, err error) {
	remote, _, err := s.linkedServer(ctx, projectID)
	if err != nil {
		return "", "", err
	}
	f, err := s.fetchServerVersion(ctx, remote.ServerProjectID, number)
	if err != nil {
		return "", "", err
	}
	return f.PixiToml, f.PixiLock, nil
}

// ServerVersion returns a version that exists on the linked server
// (server_only items included) as a VersionDetail.
func (s *Service) ServerVersion(ctx context.Context, projectID uuid.UUID, number int) (*shared.VersionDetail, error) {
	remote, _, err := s.linkedServer(ctx, projectID)
	if err != nil {
		return nil, err
	}
	f, err := s.fetchServerVersion(ctx, remote.ServerProjectID, number)
	if err != nil {
		return nil, err
	}
	item := serverOnlyItem(&f.syncVersion)
	if _, err := s.local.LoadVersion(ctx, projectID, number); err == nil {
		item.ServerOnly = false
	}
	detail := &shared.VersionDetail{VersionItem: item, Spec: artifact.Spec{Channels: []string{}, Platforms: []string{}, Requested: []artifact.RequestedPackage{}}}
	if spec, err := artifact.SpecFromManifest([]byte(f.PixiToml)); err == nil && spec != nil {
		detail.Spec = *spec
	}
	return detail, nil
}

// DecorateProjects fills UpdateAvailable, ServerLatestVersion and the server
// entry of Remotes. Registered in sdk.New. Must not fail when the server is
// unreachable or not connected.
func (s *Service) DecorateProjects(ctx context.Context, items []shared.ProjectListItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	db := s.deps.DB.WithContext(ctx)
	var remotes []models.ProjectRemote
	if err := db.Where("project_id IN ? AND kind = ?", ids, models.RemoteKindServer).Order("created_at ASC").Find(&remotes).Error; err != nil {
		s.deps.Logger.Debug("decorate projects: loading remotes", "error", err)
		return nil
	}
	byProject := map[uuid.UUID]models.ProjectRemote{}
	for _, r := range remotes {
		if _, ok := byProject[r.ProjectID]; !ok {
			byProject[r.ProjectID] = r
		}
	}
	conn, _ := s.local.LoadServerCredentials(ctx)

	type job struct {
		idx    int
		remote models.ProjectRemote
	}
	var jobs []job
	for i := range items {
		refs := make([]shared.RemoteRef, 0, len(items[i].Remotes)+1)
		r, linked := byProject[items[i].ID]
		if linked {
			name := r.Name
			if name == "" {
				name = ServerName(r.ServerURL)
			}
			refs = append(refs, shared.RemoteRef{Kind: models.RemoteKindServer, Name: name, URL: r.ServerURL, ServerProjectID: r.ServerProjectID})
		}
		for _, ref := range items[i].Remotes {
			if ref.Kind != models.RemoteKindServer {
				refs = append(refs, ref)
			}
		}
		items[i].Remotes = refs
		if linked && conn != nil && conn.URL != "" && sameServer(r.ServerURL, conn.URL) {
			jobs = append(jobs, job{idx: i, remote: r})
		}
	}
	if len(jobs) == 0 {
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx, decorateTimeout)
	defer cancel()
	states := make([]*serverState, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for k, j := range jobs {
		wg.Add(1)
		go func(k int, j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			st, err := s.fetchServerState(cctx, conn.URL, j.remote.ServerProjectID, true)
			if err != nil {
				s.deps.Logger.Debug("decorate projects: server versions", "project", j.remote.ProjectID, "error", err)
				return
			}
			states[k] = st
		}(k, j)
	}
	wg.Wait()

	for k, j := range jobs {
		st := states[k]
		if st == nil || st.max == 0 {
			continue
		}
		latest := st.max
		items[j.idx].ServerLatestVersion = &latest
		locals, err := localVersions(db, items[j.idx].ID)
		if err != nil {
			continue
		}
		have := map[int]string{}
		for _, v := range locals {
			have[v.row.VersionNumber] = v.hash
		}
		for _, sv := range st.versions {
			if h, ok := have[sv.Number]; !ok || h != sv.ContentHash {
				items[j.idx].UpdateAvailable = true
				break
			}
		}
	}
	return nil
}

// DecorateVersions sets Sync markers, appends server-only versions and
// server publications. Registered in sdk.New. Must not fail when the server
// is unreachable or not connected.
func (s *Service) DecorateVersions(ctx context.Context, projectID uuid.UUID, items []shared.VersionItem) ([]shared.VersionItem, error) {
	remote, err := s.serverRemote(ctx, projectID)
	if err != nil || remote == nil {
		return items, nil
	}
	conn, err := s.local.LoadServerCredentials(ctx)
	if err != nil || conn.URL == "" || !sameServer(remote.ServerURL, conn.URL) {
		return items, nil
	}
	cctx, cancel := context.WithTimeout(ctx, decorateTimeout)
	defer cancel()
	st, err := s.fetchServerState(cctx, conn.URL, remote.ServerProjectID, true)
	if err != nil {
		s.deps.Logger.Debug("decorate versions: server versions", "project", projectID, "error", err)
		return items, nil
	}
	return s.decorateWithState(ctx, projectID, items, st)
}

// decorateWithState applies a server state to the version items.
func (s *Service) decorateWithState(ctx context.Context, projectID uuid.UUID, items []shared.VersionItem, st *serverState) ([]shared.VersionItem, error) {
	db := s.deps.DB.WithContext(ctx)
	locals, err := localVersions(db, projectID)
	if err != nil {
		return items, nil
	}
	if plan := renumberPlan(locals, st); plan != nil {
		if err := s.applyRenumber(ctx, projectID, plan); err == nil {
			for i := range items {
				if items[i].ServerOnly {
					continue
				}
				if n, ok := plan[items[i].Number]; ok {
					items[i].Number = n
				}
				if items[i].BaseVersion != nil {
					if n, ok := plan[*items[i].BaseVersion]; ok {
						b := n
						items[i].BaseVersion = &b
					}
				}
			}
			if locals, err = localVersions(db, projectID); err != nil {
				return items, nil
			}
		} else {
			s.deps.Logger.Debug("decorate versions: renumber skipped", "project", projectID, "error", err)
		}
	}

	byNumber := map[int]localVersion{}
	localMax := 0
	for _, v := range locals {
		byNumber[v.row.VersionNumber] = v
		if v.row.VersionNumber > localMax {
			localMax = v.row.VersionNumber
		}
	}

	out := make([]shared.VersionItem, 0, len(items)+len(st.versions))
	onServer := map[int]bool{}
	for _, it := range items {
		if it.ServerOnly {
			continue // rebuilt below
		}
		it.Sync = nil
		if lv, ok := byNumber[it.Number]; ok && st.onServer(lv) {
			onServer[it.Number] = true
			it.Published = serverPublished(st.byNumber[it.Number].Publications, it.Published)
		}
		out = append(out, it)
	}
	for i := range st.versions {
		sv := &st.versions[i]
		if _, ok := byNumber[sv.Number]; ok {
			continue
		}
		out = append(out, serverOnlyItem(sv))
	}
	reconcileTags(out, onServer, st)

	if st.max > 0 {
		mark := func(n int, m shared.SyncMarker) {
			for i := range out {
				if out[i].Number == n {
					mm := m
					out[i].Sync = &mm
					return
				}
			}
		}
		// Prototype rule (InSync, ServerAhead): when both sides have the
		// same newest version it is "In sync"; otherwise the newest version
		// on this machine is "Newest here" and the server's newest version
		// (local or server-only) is "Newest on server".
		newestLocal, hasLocal := byNumber[localMax]
		switch {
		case hasLocal && localMax == st.max && st.onServer(newestLocal):
			mark(localMax, shared.SyncInSync)
		default:
			if hasLocal {
				mark(localMax, shared.SyncNewestHere)
			}
			if !hasLocal || localMax != st.max {
				mark(st.max, shared.SyncNewestOnServer)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	return out, nil
}

// reconcileTags makes the tags shown on a linked project's merged version
// list consistent (a tag names one version):
//   - "latest" is shown only on the newest version, local or server-only
//     (prototype ServerAhead: the pulled version loses "latest" to the
//     newer server-only one).
//   - A tag on a version that is not on the server yet is this machine's
//     newest intent; it stays and is hidden elsewhere.
//   - Otherwise the server's tags are authoritative for shared versions: a
//     tag the server put on another version is hidden here, and the
//     server's tags of a shared version are shown on it.
//
// items are local items (onServer says which are on the server with the
// same content) plus server-only items.
func reconcileTags(items []shared.VersionItem, onServer map[int]bool, st *serverState) {
	newest := 0
	for _, it := range items {
		newest = max(newest, it.Number)
	}
	serverTagAt := map[string]int{}
	for _, sv := range st.versions {
		for _, t := range visibleTags(sv.Tags) {
			if t != local.LatestTag {
				serverTagAt[t] = sv.Number
			}
		}
	}
	localOnlyTagAt := map[string]int{}
	for _, it := range items {
		if it.ServerOnly || onServer[it.Number] {
			continue
		}
		for _, t := range it.Tags {
			if t != local.LatestTag {
				localOnlyTagAt[t] = it.Number
			}
		}
	}
	owner := func(t string) (int, bool) {
		if n, ok := localOnlyTagAt[t]; ok {
			return n, true
		}
		n, ok := serverTagAt[t]
		return n, ok
	}
	for i := range items {
		it := &items[i]
		onBoth := it.ServerOnly || onServer[it.Number]
		tags := make([]string, 0, len(it.Tags)+1)
		seen := map[string]bool{}
		add := func(t string) {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
		for _, t := range it.Tags {
			if t == local.LatestTag {
				continue // placed below
			}
			if n, ok := owner(t); ok && n != it.Number {
				continue
			}
			add(t)
		}
		if onBoth {
			for t, n := range serverTagAt {
				if o, _ := owner(t); n == it.Number && o == it.Number {
					add(t)
				}
			}
		}
		if it.Number == newest {
			add(local.LatestTag)
		}
		sortTagsLatestFirst(tags)
		it.Tags = tags
	}
}
