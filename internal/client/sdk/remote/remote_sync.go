package remote

// Owner: U5 (remote). Helpers shared by pull, push and the sync
// decorators: the linked server project, a short-lived cache of server
// version lists, local content hashes, storing server versions locally and
// renumbering local-only versions that collide with server numbers.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// serverCacheTTL bounds how long a server version list is reused by the
// list decorators (the projects list asks for every linked project).
const serverCacheTTL = 5 * time.Second

// decorateTimeout bounds the server calls made while decorating lists, so
// an unreachable server never stalls the local UI.
const decorateTimeout = 4 * time.Second

// errProjectBusy means a job is running on the project, so its version
// numbers must not change now.
var errProjectBusy = &shared.ConflictError{
	Code:    "job_active",
	Message: "A job is running on this project. Try again when it finishes.",
}

// serverState is a linked server project's versions (oldest first).
type serverState struct {
	versions []syncVersion
	byNumber map[int]*syncVersion
	max      int
}

func newServerState(list []syncVersion) *serverState {
	sort.Slice(list, func(i, j int) bool { return list[i].Number < list[j].Number })
	st := &serverState{versions: list, byNumber: make(map[int]*syncVersion, len(list))}
	for i := range list {
		st.byNumber[list[i].Number] = &list[i]
		if list[i].Number > st.max {
			st.max = list[i].Number
		}
	}
	return st
}

type cacheKey struct{ base, project string }

type cacheEntry struct {
	at    time.Time
	state *serverState
}

var serverCache = struct {
	mu sync.Mutex
	m  map[cacheKey]cacheEntry
}{m: map[cacheKey]cacheEntry{}}

// resetServerCache forgets every cached server state (connect/disconnect).
func resetServerCache() {
	serverCache.mu.Lock()
	defer serverCache.mu.Unlock()
	serverCache.m = map[cacheKey]cacheEntry{}
}

// forgetServerProject drops one cached server state (after push/pull).
func forgetServerProject(base, serverProjectID string) {
	serverCache.mu.Lock()
	defer serverCache.mu.Unlock()
	delete(serverCache.m, cacheKey{base, serverProjectID})
}

// connection returns the stored connection or shared.ErrNotConnected.
func (s *Service) connection(ctx context.Context) (*local.ServerCredentials, error) {
	c, err := s.local.LoadServerCredentials(ctx)
	if err != nil {
		return nil, err
	}
	if c.URL == "" {
		return nil, shared.ErrNotConnected
	}
	return c, nil
}

// serverRemote returns the project's server remote, or nil when the
// project is not linked to a server.
func (s *Service) serverRemote(ctx context.Context, projectID uuid.UUID) (*models.ProjectRemote, error) {
	var r models.ProjectRemote
	err := s.deps.DB.WithContext(ctx).
		Where("project_id = ? AND kind = ?", projectID, models.RemoteKindServer).
		Order("created_at ASC").
		First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// sameServer compares two stored base URLs.
func sameServer(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
}

// fetchServerState lists a server project's versions. useCache reuses a
// list younger than serverCacheTTL.
func (s *Service) fetchServerState(ctx context.Context, base, serverProjectID string, useCache bool) (*serverState, error) {
	key := cacheKey{base, serverProjectID}
	if useCache {
		serverCache.mu.Lock()
		e, ok := serverCache.m[key]
		serverCache.mu.Unlock()
		if ok && time.Since(e.at) < serverCacheTTL {
			return e.state, nil
		}
	}
	var list []syncVersion
	if err := s.ServerJSON(ctx, http.MethodGet, "/projects/"+url.PathEscape(serverProjectID)+"/sync/versions", nil, &list); err != nil {
		return nil, err
	}
	st := newServerState(list)
	serverCache.mu.Lock()
	serverCache.m[key] = cacheEntry{at: time.Now(), state: st}
	serverCache.mu.Unlock()
	return st, nil
}

// fetchServerVersion downloads one server version with its files.
func (s *Service) fetchServerVersion(ctx context.Context, serverProjectID string, number int) (*syncVersionFiles, error) {
	var f syncVersionFiles
	path := fmt.Sprintf("/projects/%s/sync/versions/%d", url.PathEscape(serverProjectID), number)
	if err := s.ServerJSON(ctx, http.MethodGet, path, nil, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// localVersion is a stored local version with its content hash.
type localVersion struct {
	row  models.ProjectVersion
	hash string
}

// localVersions loads the project's versions (ascending) with content
// hashes computed the way the server computes them.
func localVersions(db *gorm.DB, projectID uuid.UUID) ([]localVersion, error) {
	var rows []models.ProjectVersion
	if err := db.Where("project_id = ?", projectID).Order("version_number ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading local versions: %w", err)
	}
	out := make([]localVersion, len(rows))
	for i := range rows {
		out[i] = localVersion{row: rows[i], hash: contenthash.Hash(rows[i].ManifestContent, rows[i].LockFileContent)}
	}
	return out, nil
}

// onServer reports whether the local version exists with the same content
// on the server.
func (st *serverState) onServer(v localVersion) bool {
	sv, ok := st.byNumber[v.row.VersionNumber]
	return ok && sv.ContentHash == v.hash
}

// renumberPlan maps local-only version numbers to the numbers they need so
// they sit right after the server's newest version (no collisions, no
// gaps). Nil when nothing has to move.
func renumberPlan(locals []localVersion, st *serverState) map[int]int {
	var only []int
	for _, v := range locals {
		if !st.onServer(v) {
			only = append(only, v.row.VersionNumber)
		}
	}
	plan := map[int]int{}
	for i, old := range only {
		if target := st.max + 1 + i; target != old {
			plan[old] = target
		}
	}
	if len(plan) == 0 {
		return nil
	}
	// Every local-only version moves together so their order is kept.
	for i, old := range only {
		plan[old] = st.max + 1 + i
	}
	return plan
}

// hasActiveJob reports whether a pending/running job exists on the project.
func hasActiveJob(db *gorm.DB, projectID uuid.UUID) (bool, error) {
	var count int64
	err := db.Model(&models.Job{}).
		Where("project_id = ? AND status IN ?", projectID, []models.JobStatus{models.JobStatusPending, models.JobStatusRunning}).
		Count(&count).Error
	return count > 0, err
}

// applyRenumber renumbers local-only versions (plan old -> new) in one
// transaction: versions, base versions, tags, jobs, local publications and
// the installed / last installed version all follow. Numbers go through
// negative placeholders so no two rows share a number mid-way.
func (s *Service) applyRenumber(ctx context.Context, projectID uuid.UUID, plan map[int]int) error {
	if len(plan) == 0 {
		return nil
	}
	type col struct {
		model any
		name  string
		scope string
	}
	cols := []col{
		{&models.ProjectVersion{}, "version_number", "project_id"},
		{&models.ProjectVersion{}, "base_version", "project_id"},
		{&models.ProjectTag{}, "version_number", "project_id"},
		{&models.Job{}, "version_number", "project_id"},
		{&models.Publication{}, "version_number", "project_id"},
		{&models.Project{}, "installed_version", "id"},
		{&models.Project{}, "last_installed_version", "id"},
	}
	return s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if busy, err := hasActiveJob(tx, projectID); err != nil {
			return err
		} else if busy {
			return errProjectBusy
		}
		move := func(from, to int) error {
			for _, c := range cols {
				if err := tx.Unscoped().Model(c.model).
					Where(c.scope+" = ? AND "+c.name+" = ?", projectID, from).
					Update(c.name, to).Error; err != nil {
					return fmt.Errorf("renumbering %s: %w", c.name, err)
				}
			}
			return nil
		}
		for old := range plan {
			if err := move(old, -old); err != nil {
				return err
			}
		}
		for old, n := range plan {
			if err := move(-old, n); err != nil {
				return err
			}
		}
		return local.ApplyTags(tx, projectID, 0, nil, s.deps.LocalUserID)
	})
}

// reconcile renumbers local-only versions that collide with (or leave a
// gap after) the server's versions. It returns the applied plan.
func (s *Service) reconcile(ctx context.Context, projectID uuid.UUID, st *serverState) (map[int]int, error) {
	locals, err := localVersions(s.deps.DB.WithContext(ctx), projectID)
	if err != nil {
		return nil, err
	}
	plan := renumberPlan(locals, st)
	if plan == nil {
		return nil, nil
	}
	if err := s.applyRenumber(ctx, projectID, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// visibleTags drops "latest" handling to ApplyTags and removes internal
// content-hash tags.
func visibleTags(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || local.IsContentHashTag(t) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// storeServerVersion saves a server version on this machine, keeping the
// server's number, author, description and creation time, and mirrors the
// server's tags onto it.
func (s *Service) storeServerVersion(tx *gorm.DB, projectID uuid.UUID, f *syncVersionFiles, now time.Time) (*models.ProjectVersion, error) {
	author := strings.TrimSpace(f.Author)
	created := f.CreatedAt
	if created.IsZero() {
		created = now
	}
	synced := now
	row := models.ProjectVersion{
		ProjectID:       projectID,
		VersionNumber:   f.Number,
		ManifestContent: f.PixiToml,
		LockFileContent: f.PixiLock,
		PackageMetadata: "[]",
		ContentHash:     contenthash.Hash(f.PixiToml, f.PixiLock),
		CreatedBy:       s.deps.LocalUserID,
		Description:     f.Description,
		BaseVersion:     f.BaseVersion,
		AuthorName:      author,
		SyncedAt:        &synced,
		CreatedAt:       created,
	}
	if err := tx.Create(&row).Error; err != nil {
		return nil, fmt.Errorf("storing version %d: %w", f.Number, err)
	}
	var tags []string
	for _, t := range visibleTags(f.Tags) {
		if t != local.LatestTag {
			tags = append(tags, t)
		}
	}
	if err := local.ApplyTags(tx, projectID, f.Number, tags, s.deps.LocalUserID); err != nil {
		return nil, err
	}
	return &row, nil
}

// versionItem builds a version rail item for a stored local version.
func (s *Service) versionItem(db *gorm.DB, projectID uuid.UUID, row *models.ProjectVersion) (*shared.VersionItem, error) {
	var tags []models.ProjectTag
	if err := db.Where("project_id = ? AND version_number = ?", projectID, row.VersionNumber).Order("tag ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	var project models.Project
	if err := db.Where("id = ?", projectID).First(&project).Error; err != nil {
		return nil, err
	}
	item := &shared.VersionItem{
		Number:      row.VersionNumber,
		Description: row.Description,
		Author:      shared.Author{Name: row.AuthorName},
		CreatedAt:   row.CreatedAt,
		Tags:        []string{},
		BaseVersion: row.BaseVersion,
		Published:   []shared.PublishedRef{},
		Installed:   project.InstalledVersion != nil && *project.InstalledVersion == row.VersionNumber,
		ContentHash: row.ContentHash,
	}
	for _, t := range tags {
		if !local.IsContentHashTag(t.Tag) {
			item.Tags = append(item.Tags, t.Tag)
		}
	}
	sortTagsLatestFirst(item.Tags)
	return item, nil
}

// serverOnlyItem is a version that exists only on the linked server.
func serverOnlyItem(v *syncVersion) shared.VersionItem {
	item := shared.VersionItem{
		Number:      v.Number,
		Description: v.Description,
		Author:      shared.Author{Name: v.Author},
		CreatedAt:   v.CreatedAt,
		Tags:        visibleTags(v.Tags),
		BaseVersion: v.BaseVersion,
		Published:   serverPublished(v.Publications, nil),
		ServerOnly:  true,
		ContentHash: v.ContentHash,
	}
	sortTagsLatestFirst(item.Tags)
	return item
}

// serverPublished converts server publication records to "Published"
// badge entries, skipping references already in have.
func serverPublished(recs []publicationRecord, have []shared.PublishedRef) []shared.PublishedRef {
	out := append([]shared.PublishedRef{}, have...)
	seen := map[string]bool{}
	for _, p := range have {
		seen[p.Reference] = true
	}
	for _, r := range recs {
		ref := r.RegistryHost + "/" + r.Repository + ":" + r.Tag
		if seen[ref] {
			continue
		}
		seen[ref] = true
		item := shared.PublishedRef{
			Registry:   r.RegistryHost,
			Repository: r.Repository,
			Tag:        r.Tag,
			Reference:  ref,
			Source:     "server",
		}
		if r.ID != nil {
			if id, err := uuid.Parse(*r.ID); err == nil {
				item.PublicationID = &id
			}
		}
		out = append(out, item)
	}
	return out
}

func sortTagsLatestFirst(tags []string) {
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i] == local.LatestTag || tags[j] == local.LatestTag {
			return tags[i] == local.LatestTag && tags[j] != local.LatestTag
		}
		return tags[i] < tags[j]
	})
}
