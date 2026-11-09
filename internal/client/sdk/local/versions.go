package local

// Owner: U2 (detail). Version list and files of local versions.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/user"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// ListVersions returns the project's versions newest first (author,
// description, tags without content-hash tags, base version, installed),
// then runs the registered VersionDecorators (published badges, sync
// markers, server-only versions).
func (s *Service) ListVersions(ctx context.Context, projectID uuid.UUID) ([]shared.VersionItem, error) {
	project, err := s.projectForVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var rows []models.ProjectVersion
	if err := s.deps.DB.WithContext(ctx).
		Select("id", "project_id", "version_number", "content_hash", "created_by", "description", "base_version", "author_name", "synced_at", "created_at").
		Where("project_id = ?", projectID).
		Order("version_number DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}
	tags, err := s.visibleVersionTags(ctx, projectID)
	if err != nil {
		return nil, err
	}
	authors, err := s.versionAuthors(ctx, rows)
	if err != nil {
		return nil, err
	}
	items := make([]shared.VersionItem, 0, len(rows))
	for i := range rows {
		items = append(items, s.buildVersionItem(project, &rows[i], tags, authors))
	}
	return s.runVersionDecorators(ctx, projectID, items)
}

// GetVersion returns one local version with its environment spec. The item
// carries the same decorator output (published, sync) as the list entry.
// Returns shared.ErrNotFound when the version is not on this machine
// (server-only versions are served by the remote SDK).
func (s *Service) GetVersion(ctx context.Context, projectID uuid.UUID, number int) (*shared.VersionDetail, error) {
	if _, err := s.projectForVersions(ctx, projectID); err != nil {
		return nil, err
	}
	v, err := s.LoadVersion(ctx, projectID, number)
	if err != nil {
		return nil, err
	}
	// The decorators need the whole list (sync markers depend on the newest
	// version), so the item is taken from ListVersions.
	items, err := s.ListVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var item *shared.VersionItem
	for i := range items {
		if items[i].Number == number && !items[i].ServerOnly {
			item = &items[i]
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("version %d: %w", number, shared.ErrNotFound)
	}
	spec, err := artifact.SpecFromManifest([]byte(v.ManifestContent))
	if err != nil {
		return nil, fmt.Errorf("version %d has an invalid pixi.toml: %w", number, err)
	}
	return &shared.VersionDetail{VersionItem: *item, Spec: *spec}, nil
}

// LoadVersion returns the stored version row (manifest and lock included).
// Returns shared.ErrNotFound when the version is not on this machine.
// Implemented by the foundation because several units use it.
func (s *Service) LoadVersion(ctx context.Context, projectID uuid.UUID, number int) (*models.ProjectVersion, error) {
	var v models.ProjectVersion
	err := s.deps.DB.WithContext(ctx).
		Where("project_id = ? AND version_number = ?", projectID, number).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("version %d: %w", number, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// IsContentHashTag reports whether tag is an internal content-hash tag
// ("sha-" + 12 hex), which VersionItem.Tags never shows.
func IsContentHashTag(tag string) bool {
	if len(tag) != 16 || !strings.HasPrefix(tag, "sha-") {
		return false
	}
	for _, r := range tag[4:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// GetVersionFile returns "pixi.toml" or "pixi.lock" of a version.
func (s *Service) GetVersionFile(ctx context.Context, projectID uuid.UUID, number int, name string) (*shared.FileContent, error) {
	if err := ValidateVersionFileName(name); err != nil {
		return nil, err
	}
	if _, err := s.projectForVersions(ctx, projectID); err != nil {
		return nil, err
	}
	v, err := s.LoadVersion(ctx, projectID, number)
	if err != nil {
		return nil, err
	}
	content := v.ManifestContent
	if name == artifact.LockFile {
		content = v.LockFileContent
	}
	return &shared.FileContent{Name: name, Content: content}, nil
}

// ValidateVersionFileName accepts the two files a version stores.
func ValidateVersionFileName(name string) error {
	if name != artifact.ManifestFile && name != artifact.LockFile {
		return &shared.ValidationError{Field: "name", Message: fmt.Sprintf("A version has %s and %s; %q is not one of them.", artifact.ManifestFile, artifact.LockFile, name)}
	}
	return nil
}

// ResolvedPackages lists resolved packages per platform from pixi.lock.
func (s *Service) ResolvedPackages(ctx context.Context, projectID uuid.UUID, number int) (*shared.ResolvedPackages, error) {
	if _, err := s.projectForVersions(ctx, projectID); err != nil {
		return nil, err
	}
	v, err := s.LoadVersion(ctx, projectID, number)
	if err != nil {
		return nil, err
	}
	return ResolvedPackagesFromFiles([]byte(v.ManifestContent), []byte(v.LockFileContent))
}

// ResolvedPackagesFromFiles builds the resolved packages view of one
// version's files: the default environment of the lock, platforms in the
// manifest's order first (those present in the lock), then any other lock
// platform. Exported so the remote SDK can serve server-only versions the
// same way.
func ResolvedPackagesFromFiles(manifest, lock []byte) (*shared.ResolvedPackages, error) {
	out := &shared.ResolvedPackages{
		Platforms: []string{},
		Packages:  map[string][]shared.ResolvedPackage{},
		TotalSize: map[string]int64{},
	}
	if len(bytes.TrimSpace(lock)) == 0 {
		return out, nil
	}
	parsed, err := artifact.ParseLock(lock)
	if err != nil {
		return nil, fmt.Errorf("reading pixi.lock: %w", err)
	}
	env := parsed.Environment("")
	if env == nil {
		return out, nil
	}
	lockPlatforms := env.Platforms()
	var order []string
	if spec, err := artifact.SpecFromManifest(manifest); err == nil {
		for _, p := range spec.Platforms {
			if slices.Contains(lockPlatforms, p) && !slices.Contains(order, p) {
				order = append(order, p)
			}
		}
	}
	for _, p := range lockPlatforms {
		if !slices.Contains(order, p) {
			order = append(order, p)
		}
	}
	for _, p := range order {
		pkgs := env.Packages[p]
		if pkgs == nil {
			pkgs = []shared.ResolvedPackage{}
		}
		out.Platforms = append(out.Platforms, p)
		out.Packages[p] = pkgs
		out.TotalSize[p] = artifact.TotalSize(pkgs)
	}
	return out, nil
}

// WriteVersionArchive writes a .tar.gz of the version's files to w and
// returns the suggested file name.
func (s *Service) WriteVersionArchive(ctx context.Context, projectID uuid.UUID, number int, w io.Writer) (string, error) {
	project, err := s.projectForVersions(ctx, projectID)
	if err != nil {
		return "", err
	}
	v, err := s.LoadVersion(ctx, projectID, number)
	if err != nil {
		return "", err
	}
	tags, err := s.visibleVersionTags(ctx, projectID)
	if err != nil {
		return "", err
	}
	in := artifact.ArchiveInput{
		Metadata: artifact.ArchiveMetadata{
			Name:        project.Name,
			Version:     number,
			Description: v.Description,
			Tags:        tags[number],
		},
		Toml: []byte(v.ManifestContent),
		Lock: []byte(v.LockFileContent),
	}
	if err := artifact.WriteArchive(w, in); err != nil {
		return "", fmt.Errorf("writing archive: %w", err)
	}
	return artifact.ArchiveFileName(project.Name, number), nil
}

// ---------------------------------------------------------------- helpers

// projectForVersions returns the project row or shared.ErrNotFound.
func (s *Service) projectForVersions(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	var p models.Project
	err := s.deps.DB.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("project %s: %w", id, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// visibleVersionTags maps version number to its visible tags: content-hash tags
// dropped, "latest" first, then alphabetical.
func (s *Service) visibleVersionTags(ctx context.Context, projectID uuid.UUID) (map[int][]string, error) {
	var rows []models.ProjectTag
	if err := s.deps.DB.WithContext(ctx).
		Where("project_id = ?", projectID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	out := map[int][]string{}
	for _, r := range rows {
		if IsContentHashTag(r.Tag) {
			continue
		}
		out[r.VersionNumber] = append(out[r.VersionNumber], r.Tag)
	}
	for n := range out {
		sort.Slice(out[n], func(i, j int) bool {
			a, b := out[n][i], out[n][j]
			if (a == "latest") != (b == "latest") {
				return a == "latest"
			}
			return a < b
		})
	}
	return out, nil
}

// versionAuthors resolves the users that created the given versions.
func (s *Service) versionAuthors(ctx context.Context, rows []models.ProjectVersion) (map[uuid.UUID]models.User, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	seen := map[uuid.UUID]bool{}
	for _, r := range rows {
		if r.CreatedBy != uuid.Nil && !seen[r.CreatedBy] {
			seen[r.CreatedBy] = true
			ids = append(ids, r.CreatedBy)
		}
	}
	out := map[uuid.UUID]models.User{}
	if len(ids) == 0 {
		return out, nil
	}
	var users []models.User
	if err := s.deps.DB.WithContext(ctx).Unscoped().Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("loading authors: %w", err)
	}
	for _, u := range users {
		out[u.ID] = u
	}
	return out, nil
}

// versionItem builds the undecorated rail row of a stored version.
func (s *Service) buildVersionItem(project *models.Project, v *models.ProjectVersion, tags map[int][]string, authors map[uuid.UUID]models.User) shared.VersionItem {
	t := tags[v.VersionNumber]
	if t == nil {
		t = []string{}
	}
	return shared.VersionItem{
		Number:      v.VersionNumber,
		Description: v.Description,
		Author:      s.versionAuthor(v, authors),
		CreatedAt:   v.CreatedAt,
		Tags:        append([]string(nil), t...),
		BaseVersion: v.BaseVersion,
		Published:   []shared.PublishedRef{},
		Installed:   project.InstalledVersion != nil && *project.InstalledVersion == v.VersionNumber,
		ContentHash: v.ContentHash,
	}
}

// versionAuthor is the display author of a version. AuthorName (a snapshot,
// e.g. the server author of a pulled version) wins; versions created on
// this machine by the local user show the OS account's name with the
// local username, which the UI uses to add "(you)".
func (s *Service) versionAuthor(v *models.ProjectVersion, authors map[uuid.UUID]models.User) shared.Author {
	u, known := authors[v.CreatedBy]
	isLocal := (s.deps.LocalUserID != uuid.Nil && v.CreatedBy == s.deps.LocalUserID) ||
		(known && u.Username == auth.LocalUsername())
	a := shared.Author{Name: v.AuthorName}
	switch {
	case isLocal:
		a.Username = auth.LocalUsername()
		if a.Name == "" {
			a.Name = localAuthorName()
		}
	case known:
		a.Username = u.Username
		if a.Name == "" {
			a.Name = u.Username
		}
	}
	if a.Name == "" {
		a.Name = "Unknown"
	}
	return a
}

var (
	localAuthorOnce sync.Once
	localAuthor     string
)

// localAuthorName is the OS account's full name, else its login name.
func localAuthorName() string {
	localAuthorOnce.Do(func() {
		localAuthor = "You"
		if u, err := user.Current(); err == nil {
			switch {
			case strings.TrimSpace(u.Name) != "":
				localAuthor = strings.TrimSpace(u.Name)
			case u.Username != "":
				localAuthor = u.Username
			}
		}
	})
	return localAuthor
}

// runVersionDecorators runs every registered VersionDecorator in order.
func (s *Service) runVersionDecorators(ctx context.Context, projectID uuid.UUID, items []shared.VersionItem) ([]shared.VersionItem, error) {
	if s.deps.Runtime == nil {
		return items, nil
	}
	for _, d := range s.deps.Runtime.VersionDecorators() {
		out, err := d.DecorateVersions(ctx, projectID, items)
		if err != nil {
			return nil, err
		}
		if out != nil {
			items = out
		}
	}
	for i := range items {
		if items[i].Tags == nil {
			items[i].Tags = []string{}
		}
		if items[i].Published == nil {
			items[i].Published = []shared.PublishedRef{}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Number > items[j].Number })
	return items, nil
}
