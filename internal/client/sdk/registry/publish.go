package registry

// Owner: U7 (publish). Publish / unpublish and the local Published badge.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// ReferenceHint is the validation copy for a malformed repository:tag.
const ReferenceHint = "Use repository:tag, e.g. nebari/project_1:5."

var (
	repositoryRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)
	tagRE        = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
	slugRE       = regexp.MustCompile(`[^a-z0-9._-]+`)
)

// Publication result statuses.
const (
	StatusPublished = "published"
	StatusFailed    = "failed"
)

// ValidReference reports whether repository and tag are acceptable OCI
// names (the prototype's rule, shared with the publish dialog).
func ValidReference(repository, tag string) bool {
	return repositoryRE.MatchString(repository) && !strings.Contains(repository, "//") &&
		!strings.HasSuffix(repository, "/") && tagRE.MatchString(tag)
}

// ParseReference splits "repository:tag" (the publish dialog's editable
// text) into its parts, validating both.
func ParseReference(ref string) (repository, tag string, err error) {
	ref = strings.TrimSpace(ref)
	i := strings.LastIndex(ref, ":")
	if i <= 0 {
		return "", "", &shared.ValidationError{Field: "targets", Message: ReferenceHint}
	}
	repository, tag = ref[:i], ref[i+1:]
	if !ValidReference(repository, tag) {
		return "", "", &shared.ValidationError{Field: "targets", Message: ReferenceHint}
	}
	return repository, tag, nil
}

// PublishDefaults returns the prefilled repository:tag per registry: the
// active publication of this version on that registry if there is one,
// else the repository this project last used on that registry with the
// version number as tag, else <namespace>/<project name> and the version
// number.
func (s *Service) PublishDefaults(ctx context.Context, projectID uuid.UUID, version int) ([]shared.PublishDefault, error) {
	project, err := s.loadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := s.local.LoadVersion(ctx, projectID, version); err != nil {
		return nil, err
	}
	var regs []models.OCIRegistry
	if err := s.db(ctx).Where("deleted_at IS NULL").Find(&regs).Error; err != nil {
		return nil, err
	}
	items, err := s.ListRegistries(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]models.OCIRegistry{}
	for _, r := range regs {
		byID[r.ID] = r
	}

	var pubs []models.Publication
	if err := s.db(ctx).Where("project_id = ? AND version_number = ? AND deleted_at IS NULL", projectID, version).
		Order("created_at").Find(&pubs).Error; err != nil {
		return nil, err
	}
	published := map[uuid.UUID]models.Publication{}
	for _, p := range pubs {
		published[p.RegistryID] = p
	}
	var remotes []models.ProjectRemote
	if err := s.db(ctx).Where("project_id = ? AND kind = ? AND registry_id IS NOT NULL", projectID, models.RemoteKindRegistry).
		Order("updated_at DESC").Find(&remotes).Error; err != nil {
		return nil, err
	}
	usedRepo := map[uuid.UUID]string{}
	for _, r := range remotes {
		if _, ok := usedRepo[*r.RegistryID]; !ok && r.Repository != "" {
			usedRepo[*r.RegistryID] = r.Repository
		}
	}

	out := make([]shared.PublishDefault, 0, len(items))
	for _, item := range items {
		d := shared.PublishDefault{RegistryID: item.ID, RegistryName: item.Name, Tag: fmt.Sprint(version)}
		switch {
		case published[item.ID].ID != uuid.Nil:
			d.Repository, d.Tag = published[item.ID].Repository, published[item.ID].Tag
		case usedRepo[item.ID] != "":
			d.Repository = usedRepo[item.ID]
		default:
			_, ns, _ := endpoint(byID[item.ID])
			d.Repository = defaultRepository(ns, project.Name)
		}
		out = append(out, d)
	}
	return out, nil
}

// defaultRepository is <namespace>/<project name as an OCI name>.
func defaultRepository(namespace, projectName string) string {
	slug := strings.Trim(slugRE.ReplaceAllString(strings.ToLower(projectName), "-"), "-._")
	if slug == "" {
		slug = "project"
	}
	if namespace == "" {
		return slug
	}
	return namespace + "/" + slug
}

// Publish pushes a version to each target and records a models.Publication
// per success (RegistryHost filled). Synchronous; one result per target,
// in request order. A failed target does not stop the others. Also reports
// publications to the linked server when connected.
func (s *Service) Publish(ctx context.Context, projectID uuid.UUID, version int, req shared.PublishRequest) (*shared.PublishResult, error) {
	if len(req.Targets) == 0 {
		return nil, &shared.ValidationError{Field: "targets", Message: "Choose at least one registry."}
	}
	project, err := s.loadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	v, err := s.local.LoadVersion(ctx, projectID, version)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(v.ManifestContent) == "" {
		return nil, &shared.ValidationError{Field: "version", Message: fmt.Sprintf("Version %d has no pixi.toml to publish.", version)}
	}

	type target struct {
		reg  models.OCIRegistry
		repo string
		tag  string
	}
	targets := make([]target, 0, len(req.Targets))
	seen := map[string]bool{}
	for _, t := range req.Targets {
		reg, err := s.loadRegistry(ctx, t.RegistryID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return nil, &shared.ValidationError{Field: "targets", Message: "That registry is no longer configured on this machine."}
			}
			return nil, err
		}
		repo, tag := strings.TrimSpace(t.Repository), strings.TrimSpace(t.Tag)
		if !ValidReference(repo, tag) {
			return nil, &shared.ValidationError{Field: "targets", Message: ReferenceHint}
		}
		key := reg.ID.String() + "/" + repo + ":" + tag
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, target{reg: *reg, repo: repo, tag: tag})
	}

	results := make([]shared.PublishResultItem, 0, len(targets))
	for _, t := range targets {
		ep, err := s.ociEndpoint(ctx, t.reg)
		item := shared.PublishResultItem{
			RegistryID:   t.reg.ID,
			RegistryName: t.reg.Name,
			Reference:    ep.Host + "/" + t.repo + ":" + t.tag,
			Status:       StatusFailed,
		}
		if err == nil {
			var res oci.PublishResult
			res, err = oci.PublishFiles(ctx, []byte(v.ManifestContent), []byte(v.LockFileContent), ep, t.repo, t.tag)
			if err == nil {
				var pub *models.Publication
				pub, err = s.recordPublication(ctx, project, version, t.reg, ep.Host, t.repo, t.tag, res.Digest)
				if err == nil {
					item.Status = StatusPublished
					item.Digest = res.Digest
					item.PublicationID = &pub.ID
					s.reportPublication(ctx, project.ID, v, pub)
				}
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			item.Error = publishErrorMessage(t.reg, ep.Host, err)
		}
		results = append(results, item)
	}
	return &shared.PublishResult{Results: results}, nil
}

// publishErrorMessage turns a push failure into a message for the publish
// results: registry rejections (missing or wrong credentials) and
// unreachable registries get plain wording, anything else keeps the
// underlying error.
func publishErrorMessage(reg models.OCIRegistry, host string, err error) string {
	if errors.Is(err, auth.ErrBasicCredentialNotFound) {
		return fmt.Sprintf("%s needs a username and password to publish. Add them on the Registries page.", reg.Name)
	}
	var resp *errcode.ErrorResponse
	if errors.As(err, &resp) {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			if reg.Username == "" {
				return fmt.Sprintf("%s needs a username and password to publish. Add them on the Registries page.", reg.Name)
			}
			return fmt.Sprintf("%s didn’t accept the username and password. Check them on the Registries page.", reg.Name)
		}
	}
	var netErr *net.OpError
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || errors.As(err, &netErr) {
		return fmt.Sprintf("Couldn’t reach %s. Check the registry URL and your connection.", host)
	}
	return err.Error()
}

// RecordPublication records that a version was published outside
// Publish (the CLI's directory publish, which bundles extra project files)
// so the version shows the Published badge. repository is the path after
// the registry host.
func (s *Service) RecordPublication(ctx context.Context, projectID uuid.UUID, version int, registryID uuid.UUID, repository, tag, digest string) (*models.Publication, error) {
	project, err := s.loadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	v, err := s.local.LoadVersion(ctx, projectID, version)
	if err != nil {
		return nil, err
	}
	reg, err := s.loadRegistry(ctx, registryID)
	if err != nil {
		return nil, err
	}
	if !ValidReference(repository, tag) {
		return nil, &shared.ValidationError{Field: "targets", Message: ReferenceHint}
	}
	host, _, _ := endpoint(*reg)
	pub, err := s.recordPublication(ctx, project, version, *reg, host, repository, tag, digest)
	if err != nil {
		return nil, err
	}
	s.reportPublication(ctx, project.ID, v, pub)
	return pub, nil
}

// recordPublication stores the publication, retires publications that the
// same registry tag pointed at before (the tag now points at this
// version), and records the repository as a registry remote of the
// project.
func (s *Service) recordPublication(ctx context.Context, project *models.Project, version int, reg models.OCIRegistry, host, repo, tag, digest string) (*models.Publication, error) {
	pub := &models.Publication{
		ProjectID:     project.ID,
		VersionNumber: version,
		RegistryID:    reg.ID,
		Repository:    repo,
		Tag:           tag,
		Digest:        digest,
		RegistryHost:  host,
		PublishedBy:   s.deps.LocalUserID,
	}
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&models.Publication{}).
			Where("registry_id = ? AND repository = ? AND tag = ? AND deleted_at IS NULL", reg.ID, repo, tag).
			Update("deleted_at", now).Error; err != nil {
			return err
		}
		if err := tx.Omit("Project", "Registry", "PublishedByUser").Create(pub).Error; err != nil {
			return err
		}
		return upsertRegistryRemote(tx, project.ID, reg.ID, host, repo)
	})
	if err != nil {
		return nil, fmt.Errorf("recording publication: %w", err)
	}
	return pub, nil
}

// upsertRegistryRemote records host/repository as a registry remote of the
// project ("Connected remotes" shows ghcr.io/example-org).
func upsertRegistryRemote(tx *gorm.DB, projectID, registryID uuid.UUID, host, repo string) error {
	name := host
	if i := strings.Index(repo, "/"); i > 0 {
		name = host + "/" + repo[:i]
	}
	var existing models.ProjectRemote
	err := tx.Where("project_id = ? AND kind = ? AND registry_host = ? AND repository = ?", projectID, models.RemoteKindRegistry, host, repo).
		First(&existing).Error
	switch {
	case err == nil:
		return tx.Model(&existing).Updates(map[string]any{"registry_id": registryID, "name": name, "updated_at": time.Now()}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		id := registryID
		return tx.Create(&models.ProjectRemote{
			ProjectID:    projectID,
			Kind:         models.RemoteKindRegistry,
			Name:         name,
			RegistryID:   &id,
			RegistryHost: host,
			Repository:   repo,
		}).Error
	default:
		return err
	}
}

// Unpublish deletes the tag from the registry when it supports tag
// deletion (else shared.ErrUnavailable) and soft-deletes the publication.
func (s *Service) Unpublish(ctx context.Context, projectID uuid.UUID, publicationID uuid.UUID) error {
	var pub models.Publication
	err := s.db(ctx).Where("id = ? AND project_id = ? AND deleted_at IS NULL", publicationID, projectID).First(&pub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("publication %s: %w", publicationID, shared.ErrNotFound)
	}
	if err != nil {
		return err
	}
	reg, err := s.loadRegistry(ctx, pub.RegistryID)
	if err != nil {
		return err
	}
	ep, err := s.ociEndpoint(ctx, *reg)
	if err != nil {
		return err
	}
	if ok, reason := oci.TagDeletionSupport(ep.Host); !ok {
		return fmt.Errorf("%s: %w", reason, shared.ErrUnavailable)
	}
	if err := oci.DeleteTag(ctx, ep, pub.Repository, pub.Tag); err != nil {
		if errors.Is(err, oci.ErrTagDeletionUnsupported) {
			return fmt.Errorf("%s can’t delete tags through the registry API: %w", reg.Name, shared.ErrUnavailable)
		}
		return err
	}
	now := time.Now()
	if err := s.db(ctx).Model(&models.Publication{}).Where("id = ?", pub.ID).Update("deleted_at", now).Error; err != nil {
		return fmt.Errorf("recording unpublish: %w", err)
	}
	if pub.RegistryHost == "" {
		pub.RegistryHost = ep.Host
	}
	s.reportUnpublish(ctx, projectID, pub)
	return nil
}

// DecorateVersions adds local publications to VersionItem.Published.
// Registered in sdk.New. Server-only versions are left to sdk/remote.
func (s *Service) DecorateVersions(ctx context.Context, projectID uuid.UUID, items []shared.VersionItem) ([]shared.VersionItem, error) {
	byVersion, err := s.PublishedRefs(ctx, projectID)
	if err != nil {
		return items, nil
	}
	for i := range items {
		if items[i].ServerOnly {
			continue
		}
		refs := byVersion[items[i].Number]
		if len(refs) == 0 {
			continue
		}
		items[i].Published = append(items[i].Published, refs...)
	}
	return items, nil
}

// PublishedRefs returns this machine's active publications of a project as
// badge entries keyed by version number, oldest first.
func (s *Service) PublishedRefs(ctx context.Context, projectID uuid.UUID) (map[int][]shared.PublishedRef, error) {
	var pubs []models.Publication
	if err := s.db(ctx).Where("project_id = ? AND deleted_at IS NULL", projectID).
		Order("created_at").Find(&pubs).Error; err != nil {
		return nil, err
	}
	if len(pubs) == 0 {
		return map[int][]shared.PublishedRef{}, nil
	}
	var regs []models.OCIRegistry
	if err := s.db(ctx).Where("deleted_at IS NULL").Find(&regs).Error; err != nil {
		return nil, err
	}
	regByID := map[uuid.UUID]models.OCIRegistry{}
	for _, r := range regs {
		regByID[r.ID] = r
	}
	out := map[int][]shared.PublishedRef{}
	for _, p := range pubs {
		p := p
		host := p.RegistryHost
		reg, configured := regByID[p.RegistryID]
		if host == "" && configured {
			host, _, _ = endpoint(reg)
		}
		can := false
		if configured {
			can, _ = oci.TagDeletionSupport(host)
		}
		ref := shared.PublishedRef{
			PublicationID: &p.ID,
			Repository:    p.Repository,
			Tag:           p.Tag,
			Registry:      host,
			Reference:     host + "/" + p.Repository + ":" + p.Tag,
			Source:        "local",
			CanUnpublish:  can,
		}
		if configured {
			id := p.RegistryID
			ref.RegistryID = &id
		}
		out[p.VersionNumber] = append(out[p.VersionNumber], ref)
	}
	return out, nil
}

func (s *Service) loadProject(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	var p models.Project
	err := s.db(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("project %s: %w", id, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ------------------------------------------------- linked server (gap 2)

// ServerPublication is the team server's publication record
// (internal/server/core.PublicationRecord; the client never imports server
// packages). The local publication ID is reused as the server record ID so
// unpublish can address it.
type ServerPublication struct {
	ID           *uuid.UUID `json:"id,omitempty"`
	RegistryHost string     `json:"registry_host"`
	Repository   string     `json:"repository"`
	Tag          string     `json:"tag"`
	Digest       string     `json:"digest,omitempty"`
	PublishedBy  string     `json:"published_by,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
}

// ServerPublicationRecord converts a local publication to the record the
// team server stores (registry host, repository, tag, digest; never
// credentials). sdk/remote can send the same shape with pushed versions.
func ServerPublicationRecord(p models.Publication, publishedBy string) ServerPublication {
	id := p.ID
	at := p.CreatedAt
	return ServerPublication{
		ID:           &id,
		RegistryHost: p.RegistryHost,
		Repository:   p.Repository,
		Tag:          p.Tag,
		Digest:       p.Digest,
		PublishedBy:  publishedBy,
		PublishedAt:  &at,
	}
}

// linkedServerProject returns the server URL, token and server project ID
// when the project is linked to the currently connected server and the
// version is known to be there. ok is false otherwise.
func (s *Service) linkedServerProject(ctx context.Context, projectID uuid.UUID, v *models.ProjectVersion) (base, token, serverProjectID string, ok bool) {
	if v != nil && v.SyncedAt == nil {
		return "", "", "", false
	}
	var cfg store.Config
	if err := s.db(ctx).First(&cfg, 1).Error; err != nil || strings.TrimSpace(cfg.ServerURL) == "" {
		return "", "", "", false
	}
	var remote models.ProjectRemote
	if err := s.db(ctx).Where("project_id = ? AND kind = ?", projectID, models.RemoteKindServer).First(&remote).Error; err != nil {
		return "", "", "", false
	}
	base = strings.TrimRight(cfg.ServerURL, "/")
	if remote.ServerProjectID == "" || (remote.ServerURL != "" && strings.TrimRight(remote.ServerURL, "/") != base) {
		return "", "", "", false
	}
	var creds store.Credentials
	_ = s.db(ctx).First(&creds, 1).Error
	return base, creds.Token, remote.ServerProjectID, true
}

// reportPublication tells the linked server about a publication. Best
// effort: a version that is not on the server yet carries its publications
// when it is pushed.
func (s *Service) reportPublication(ctx context.Context, projectID uuid.UUID, v *models.ProjectVersion, pub *models.Publication) {
	base, token, sid, ok := s.linkedServerProject(ctx, projectID, v)
	if !ok {
		return
	}
	var user models.User
	_ = s.db(ctx).Where("id = ?", s.deps.LocalUserID).First(&user).Error
	path := fmt.Sprintf("/projects/%s/sync/versions/%d/publications", sid, pub.VersionNumber)
	if err := s.serverCall(ctx, base, token, http.MethodPost, path, ServerPublicationRecord(*pub, user.Username)); err != nil {
		s.logWarn("reporting publication to the server", err)
	}
}

// reportUnpublish tells the linked server a publication was removed. The
// server may know the publication under its own ID (records sent with a
// pushed version get server IDs), so when the local ID is unknown there the
// active server records of that version with the same registry host,
// repository and tag are removed instead.
func (s *Service) reportUnpublish(ctx context.Context, projectID uuid.UUID, pub models.Publication) {
	base, token, sid, ok := s.linkedServerProject(ctx, projectID, nil)
	if !ok {
		return
	}
	versionPath := fmt.Sprintf("/projects/%s/sync/versions/%d", sid, pub.VersionNumber)
	err := s.serverCall(ctx, base, token, http.MethodDelete, fmt.Sprintf("%s/publications/%s", versionPath, pub.ID), nil)
	var status *serverStatusError
	if err == nil || !errors.As(err, &status) || status.code != http.StatusNotFound {
		if err != nil {
			s.logWarn("reporting unpublish to the server", err)
		}
		return
	}
	var version struct {
		Publications []ServerPublication `json:"publications"`
	}
	if err := s.serverGet(ctx, base, token, versionPath, &version); err != nil {
		s.logWarn("reporting unpublish to the server", err)
		return
	}
	host := pub.RegistryHost
	for _, rec := range version.Publications {
		if rec.ID == nil || rec.RegistryHost != host || rec.Repository != pub.Repository || rec.Tag != pub.Tag {
			continue
		}
		if err := s.serverCall(ctx, base, token, http.MethodDelete, fmt.Sprintf("%s/publications/%s", versionPath, *rec.ID), nil); err != nil {
			s.logWarn("reporting unpublish to the server", err)
		}
	}
}

// serverStatusError is a non-2xx response from the linked server.
type serverStatusError struct{ code int }

func (e *serverStatusError) Error() string { return fmt.Sprintf("server returned %d", e.code) }

func (s *Service) serverCall(ctx context.Context, base, token, method, path string, body any) error {
	return s.serverDo(ctx, base, token, method, path, body, nil)
}

func (s *Service) serverGet(ctx context.Context, base, token, path string, out any) error {
	return s.serverDo(ctx, base, token, http.MethodGet, path, nil, out)
}

func (s *Service) serverDo(ctx context.Context, base, token, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, base+"/api/v1"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := s.deps.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 4<<20)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, limited)
		return &serverStatusError{code: resp.StatusCode}
	}
	if out != nil {
		return json.NewDecoder(limited).Decode(out)
	}
	_, _ = io.Copy(io.Discard, limited)
	return nil
}

func (s *Service) logWarn(msg string, err error) {
	if s.deps.Logger != nil {
		s.deps.Logger.Warn(msg, "error", err)
	}
}
