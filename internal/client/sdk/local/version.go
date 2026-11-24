package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"gorm.io/gorm"
)

// VersionOrigin records how a version entered this machine's history.
type VersionOrigin string

const (
	OriginCreated  VersionOrigin = "created"  // CreateProject and CreateVersion, solved by nebi
	OriginResolved VersionOrigin = "resolved" // CreateResolvedVersion, solved outside nebi
	OriginPulled   VersionOrigin = "pulled"   // written by sdk/remote on pull
)

// EnvironmentSpecification is a pixi.toml together with the pixi.lock
// solved from it: everything needed to reproduce one environment. Two
// specifications with the same Hash are the same environment, on any
// machine.
type EnvironmentSpecification struct {
	Manifest string `gorm:"type:text;not null" json:"-"` // pixi.toml
	Lock     string `gorm:"type:text;not null" json:"-"` // pixi.lock
}

// Hash is the content hash of the specification, the identity of its
// environment across machines.
func (e EnvironmentSpecification) Hash() string {
	return contenthash.Hash(e.Manifest, e.Lock)
}

// ProjectVersion is an immutable environment specification of a project
// with its history. No column is updated after insert: every change to a
// project's environment is a new version.
//
// Every version is complete. Its specification never depends on another
// version, so reading, installing, pushing or publishing a version reads
// that one row.
type ProjectVersion struct {
	ID        uuid.UUID `gorm:"type:text;primaryKey" json:"id"`
	ProjectID uuid.UUID `gorm:"type:text;not null;uniqueIndex:idx_client_version_number" json:"project_id"`

	// Number is a per-machine display number, assigned in the insert
	// transaction as MAX(number)+1. It is never reused. It is not an
	// identity across machines: use ContentHash for that.
	Number int `gorm:"not null;uniqueIndex:idx_client_version_number" json:"number"`

	// BaseVersionID is the version this one was created from. Lineage
	// only: the specification is complete without it. Nil for the first
	// version, resolved versions without an installed version, and
	// imports.
	BaseVersionID *uuid.UUID `gorm:"type:text;index" json:"base_version_id,omitempty"`

	// EnvironmentSpecification is the version's pixi.toml and pixi.lock,
	// served by GetVersionFile. Its columns are manifest and lock.
	EnvironmentSpecification `gorm:"embedded"`

	// ContentHash is EnvironmentSpecification.Hash(), stored so versions
	// can be looked up by it. Two versions on different machines with the
	// same hash are the same environment.
	ContentHash string `gorm:"type:text;not null;index" json:"content_hash"`

	Origin      VersionOrigin `gorm:"not null" json:"origin"`
	Author      string        `gorm:"not null" json:"author"`
	Description string        `gorm:"type:text" json:"description,omitempty"`

	CreatedAt time.Time `json:"created_at"`

	// Project gives the table its foreign key to client_projects. It is
	// never loaded or written.
	Project *Project `gorm:"foreignKey:ProjectID" json:"-"`
}

// TableName keeps the client table apart from the server's versions table.
func (ProjectVersion) TableName() string { return "client_project_versions" }

// BeforeCreate assigns an ID when none is set.
func (v *ProjectVersion) BeforeCreate(tx *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return nil
}

// ProjectTag is a movable name pointing at one version of a project.
// "latest" is reserved and never stored: it resolves to the highest
// Number, so it cannot drift. Names starting with "sha-" are reserved for
// content-hash tags created by push and publish.
type ProjectTag struct {
	ProjectID uuid.UUID `gorm:"type:text;primaryKey" json:"project_id"`
	Name      string    `gorm:"primaryKey" json:"name"`
	VersionID uuid.UUID `gorm:"type:text;not null;index" json:"version_id"`
	UpdatedAt time.Time `json:"updated_at"`

	// Project and Version give the table its foreign keys. They are never
	// loaded or written.
	Project *Project        `gorm:"foreignKey:ProjectID" json:"-"`
	Version *ProjectVersion `gorm:"foreignKey:VersionID" json:"-"`
}

// TableName keeps the client table apart from the server's tags table.
func (ProjectTag) TableName() string { return "client_project_tags" }

// LatestTag is the reserved tag that always names the highest Number.
const LatestTag = "latest"

// ProjectVersionSummary is a version as the version list and GetVersion
// return it. Manifest and Lock are not loaded; GetVersionFile serves them.
type ProjectVersionSummary struct {
	ProjectVersion
	BaseNumber *int     `json:"base_number,omitempty"`
	Tags       []string `json:"tags"` // includes "latest" on the highest Number
	Installed  bool     `json:"installed"`
}

// WorkingCopyStatus compares pixi.toml and pixi.lock in the project
// directory with the installed version.
type WorkingCopyStatus string

const (
	WorkingCopyClean    WorkingCopyStatus = "clean"    // files match the installed version
	WorkingCopyModified WorkingCopyStatus = "modified" // files differ, e.g. hand-edited pixi.toml
	WorkingCopyMissing  WorkingCopyStatus = "missing"  // directory or pixi.toml is gone
	WorkingCopyNoBase   WorkingCopyStatus = "no_base"  // nothing installed to compare with
)

// VersionFile names one of the two files a version holds.
type VersionFile string

const (
	FileManifest VersionFile = artifact.ManifestFile
	FileLock     VersionFile = artifact.LockFile
)

// summaryColumns are the version columns read for summaries: everything
// but the file contents, which can be several MB per version.
var summaryColumns = []string{"id", "project_id", "number", "base_version_id", "content_hash", "origin", "author", "description", "created_at"}

// tagPattern is the OCI distribution spec's tag grammar. Server publish
// sends project tags as OCI tags, so a tag that is not a valid OCI tag
// would break publish later.
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)

// validateTagName checks a tag name against the OCI tag grammar and the
// reserved names. "latest" and the "sha-" prefix are refused in any case,
// so "Latest" cannot be mistaken for the reserved tag.
func validateTagName(name string) error {
	if !tagPattern.MatchString(name) {
		return fmt.Errorf("%w: %q: use up to 128 letters, digits, '_', '.' and '-', not starting with '.' or '-'", ErrInvalidTag, name)
	}
	lower := strings.ToLower(name)
	if lower == LatestTag {
		return fmt.Errorf("%w: %q is reserved: it always names the newest version", ErrInvalidTag, name)
	}
	if strings.HasPrefix(lower, "sha-") {
		return fmt.Errorf("%w: %q: the \"sha-\" prefix is reserved for content-hash tags", ErrInvalidTag, name)
	}
	return nil
}

// checkTags trims tags, drops empty and repeated ones, and validates the
// rest.
func checkTags(tags []string) ([]string, error) {
	tags = cleanTags(tags)
	for _, t := range tags {
		if err := validateTagName(t); err != nil {
			return nil, err
		}
	}
	return tags, nil
}

// recordVersionRequest is everything recordVersion needs to write one
// version.
type recordVersionRequest struct {
	ProjectID uuid.UUID
	// BaseVersionID is the version this one was created from; nil for
	// version 1, resolved versions without an installed version, and
	// imports.
	BaseVersionID *uuid.UUID
	// EnvironmentSpecification is the complete pixi.toml and pixi.lock;
	// both are required.
	EnvironmentSpecification
	Origin VersionOrigin
	Author string
	// Description is the version's description, not the workspace's.
	Description string
	Tags        []string
}

// versionTx runs fn in a transaction while holding the lock that
// serializes every version write in this process. Number is MAX(number)+1
// read in the insert transaction, so two writers must not interleave
// between that read and the commit; callers of recordVersion go through
// here. Other processes write through the job runner of the one process
// that owns the data directory (see jobs.Owner), and the unique index on
// (project_id, number) refuses a duplicate number from any writer.
func (s *Service) versionTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	s.versionMu.Lock()
	defer s.versionMu.Unlock()
	return s.db.WithContext(ctx).Transaction(fn)
}

// recordVersion validates the tags, computes ContentHash, assigns Number
// as MAX(number)+1, and inserts the version and its tags in tx. It is the
// only code that writes client_project_versions. Call it inside versionTx.
//
// Every call records a new version, even when another version of the
// project has the same content.
func (s *Service) recordVersion(tx *gorm.DB, req recordVersionRequest) (*ProjectVersion, error) {
	tags, err := checkTags(req.Tags)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Manifest) == "" {
		return nil, fmt.Errorf("%w: a version needs a pixi.toml", ErrInvalidManifest)
	}
	// Every version is complete: without a lock it could not be installed
	// as recorded.
	if strings.TrimSpace(req.Lock) == "" {
		return nil, fmt.Errorf("%w: a version needs a pixi.lock", ErrInvalidRequest)
	}
	switch req.Origin {
	case OriginCreated, OriginResolved, OriginPulled:
	default:
		return nil, fmt.Errorf("unknown version origin %q", req.Origin)
	}

	var projects int64
	if err := tx.Model(&Project{}).Where("id = ?", req.ProjectID).Count(&projects).Error; err != nil {
		return nil, fmt.Errorf("checking project %s: %w", req.ProjectID, err)
	}
	if projects == 0 {
		return nil, fmt.Errorf("%w: project %s", ErrNotFound, req.ProjectID)
	}
	if req.BaseVersionID != nil {
		var bases int64
		if err := tx.Model(&ProjectVersion{}).Where("id = ? AND project_id = ?", *req.BaseVersionID, req.ProjectID).Count(&bases).Error; err != nil {
			return nil, fmt.Errorf("checking base version: %w", err)
		}
		if bases == 0 {
			return nil, fmt.Errorf("%w: base version %s of project %s", ErrNotFound, *req.BaseVersionID, req.ProjectID)
		}
	}
	if err := tagsFree(tx, req.ProjectID, uuid.Nil, tags); err != nil {
		return nil, err
	}

	var maxNumber int
	if err := tx.Model(&ProjectVersion{}).Where("project_id = ?", req.ProjectID).
		Select("COALESCE(MAX(number), 0)").Scan(&maxNumber).Error; err != nil {
		return nil, fmt.Errorf("numbering the version: %w", err)
	}
	v := &ProjectVersion{
		ProjectID:                req.ProjectID,
		Number:                   maxNumber + 1,
		BaseVersionID:            req.BaseVersionID,
		EnvironmentSpecification: req.EnvironmentSpecification,
		ContentHash:              req.Hash(),
		Origin:                   req.Origin,
		Author:                   req.Author,
		Description:              req.Description,
	}
	if err := tx.Create(v).Error; err != nil {
		return nil, fmt.Errorf("inserting version %d: %w", v.Number, err)
	}
	for _, name := range tags {
		if err := tx.Create(&ProjectTag{ProjectID: req.ProjectID, Name: name, VersionID: v.ID}).Error; err != nil {
			return nil, fmt.Errorf("tagging version %d %q: %w", v.Number, name, err)
		}
	}
	return v, nil
}

// tagsFree returns ErrTagExists when one of names already points at a
// version of the project other than versionID (uuid.Nil: any version).
func tagsFree(tx *gorm.DB, projectID, versionID uuid.UUID, names []string) error {
	if len(names) == 0 {
		return nil
	}
	var taken []ProjectTag
	if err := tx.Where("project_id = ? AND name IN ?", projectID, names).Find(&taken).Error; err != nil {
		return fmt.Errorf("checking tags: %w", err)
	}
	for _, t := range taken {
		if t.VersionID != versionID {
			return fmt.Errorf("%w: %q is on another version; move it with SetTag", ErrTagExists, t.Name)
		}
	}
	return nil
}

// latestVersion returns the project's version with the highest Number,
// reading only cols (all when none are given), or nil when there is none.
func latestVersion(tx *gorm.DB, projectID uuid.UUID, cols ...string) (*ProjectVersion, error) {
	var list []ProjectVersion
	q := tx.Where("project_id = ?", projectID).Order("number DESC").Limit(1)
	if len(cols) > 0 {
		q = q.Select(cols)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, fmt.Errorf("reading the latest version: %w", err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

// versionByID returns a whole version of the project, or ErrNotFound.
func versionByID(tx *gorm.DB, projectID, id uuid.UUID) (*ProjectVersion, error) {
	var v ProjectVersion
	err := tx.Where("id = ? AND project_id = ?", id, projectID).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: version %s of project %s", ErrNotFound, id, projectID)
	}
	if err != nil {
		return nil, fmt.Errorf("reading version %s: %w", id, err)
	}
	return &v, nil
}

// ListVersions returns the project's versions, newest first, with their
// tags ("latest" on the highest number), base number, origin and whether
// each is the installed one. File contents are left out; GetVersionFile
// serves them.
func (s *Service) ListVersions(ctx context.Context, projectID uuid.UUID) ([]ProjectVersionSummary, error) {
	var out []ProjectVersionSummary
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := projectIn(tx, projectID)
		if err != nil {
			return err
		}
		var list []ProjectVersion
		if err := tx.Select(summaryColumns).Where("project_id = ?", projectID).Order("number DESC").Find(&list).Error; err != nil {
			return fmt.Errorf("listing versions: %w", err)
		}
		out, err = summarize(tx, p, list)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetVersion returns one version of the project by number, like an entry
// of ListVersions.
func (s *Service) GetVersion(ctx context.Context, projectID uuid.UUID, number int) (*ProjectVersionSummary, error) {
	var out *ProjectVersionSummary
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := projectIn(tx, projectID)
		if err != nil {
			return err
		}
		var list []ProjectVersion
		if err := tx.Select(summaryColumns).Where("project_id = ? AND number = ?", projectID, number).Find(&list).Error; err != nil {
			return fmt.Errorf("reading version %d: %w", number, err)
		}
		if len(list) == 0 {
			return fmt.Errorf("%w: version %d of project %s", ErrNotFound, number, p.Name)
		}
		sums, err := summarize(tx, p, list)
		if err != nil {
			return err
		}
		out = &sums[0]
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetVersionFile returns the raw pixi.toml or pixi.lock of a version.
func (s *Service) GetVersionFile(ctx context.Context, projectID uuid.UUID, number int, file VersionFile) (string, error) {
	var col string
	switch file {
	case FileManifest:
		col = "manifest"
	case FileLock:
		col = "lock"
	default:
		return "", fmt.Errorf("unknown version file %q: use %q or %q", file, FileManifest, FileLock)
	}
	p, err := s.loadProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	var list []ProjectVersion
	if err := s.db.WithContext(ctx).Select("id", col).Where("project_id = ? AND number = ?", projectID, number).Find(&list).Error; err != nil {
		return "", fmt.Errorf("reading version %d: %w", number, err)
	}
	if len(list) == 0 {
		return "", fmt.Errorf("%w: version %d of project %s", ErrNotFound, number, p.Name)
	}
	if file == FileManifest {
		return list[0].Manifest, nil
	}
	return list[0].Lock, nil
}

// ResolveTag returns the whole version a tag points at. "latest" resolves
// to the version with the highest number.
func (s *Service) ResolveTag(ctx context.Context, projectID uuid.UUID, tag string) (*ProjectVersion, error) {
	var out *ProjectVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := projectIn(tx, projectID)
		if err != nil {
			return err
		}
		if tag == LatestTag {
			v, err := latestVersion(tx, projectID)
			if err != nil {
				return err
			}
			if v == nil {
				return fmt.Errorf("%w: project %s has no versions", ErrNotFound, p.Name)
			}
			out = v
			return nil
		}
		var t ProjectTag
		err = tx.Where("project_id = ? AND name = ?", projectID, tag).First(&t).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: tag %q of project %s", ErrNotFound, tag, p.Name)
		}
		if err != nil {
			return fmt.Errorf("reading tag %q: %w", tag, err)
		}
		out, err = versionByID(tx, projectID, t.VersionID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetTagRequest points a tag at a version.
type SetTagRequest struct {
	ProjectID uuid.UUID
	Name      string
	VersionID uuid.UUID
	// Move must be true to re-point a tag that already exists on another
	// version. Without it, SetTag returns ErrTagExists.
	Move bool
}

// SetTag creates a tag on a version of the project, or moves an existing
// one when req.Move is set. Setting a tag on the version it already points
// at changes nothing. Tags are local to this machine.
func (s *Service) SetTag(ctx context.Context, req SetTagRequest) (*ProjectTag, error) {
	if err := validateTagName(req.Name); err != nil {
		return nil, err
	}
	var out ProjectTag
	err := s.versionTx(ctx, func(tx *gorm.DB) error {
		if _, err := projectIn(tx, req.ProjectID); err != nil {
			return err
		}
		if _, err := versionByIDCols(tx, req.ProjectID, req.VersionID); err != nil {
			return err
		}
		var existing []ProjectTag
		if err := tx.Where("project_id = ? AND name = ?", req.ProjectID, req.Name).Find(&existing).Error; err != nil {
			return fmt.Errorf("reading tag %q: %w", req.Name, err)
		}
		if len(existing) == 0 {
			out = ProjectTag{ProjectID: req.ProjectID, Name: req.Name, VersionID: req.VersionID}
			if err := tx.Create(&out).Error; err != nil {
				return fmt.Errorf("creating tag %q: %w", req.Name, err)
			}
			return nil
		}
		out = existing[0]
		if out.VersionID == req.VersionID {
			return nil
		}
		if !req.Move {
			return fmt.Errorf("%w: %q is on another version; pass Move to re-point it", ErrTagExists, req.Name)
		}
		out.VersionID = req.VersionID
		if err := tx.Model(&ProjectTag{}).Where("project_id = ? AND name = ?", req.ProjectID, req.Name).
			Updates(map[string]any{"version_id": req.VersionID, "updated_at": time.Now()}).Error; err != nil {
			return fmt.Errorf("moving tag %q: %w", req.Name, err)
		}
		return tx.Where("project_id = ? AND name = ?", req.ProjectID, req.Name).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTag removes a tag. The version it pointed at stays. "latest" is not
// stored and cannot be deleted.
func (s *Service) DeleteTag(ctx context.Context, projectID uuid.UUID, name string) error {
	if strings.EqualFold(name, LatestTag) {
		return fmt.Errorf("%w: %q is reserved and cannot be deleted", ErrInvalidTag, name)
	}
	return s.versionTx(ctx, func(tx *gorm.DB) error {
		p, err := projectIn(tx, projectID)
		if err != nil {
			return err
		}
		res := tx.Where("project_id = ? AND name = ?", projectID, name).Delete(&ProjectTag{})
		if res.Error != nil {
			return fmt.Errorf("deleting tag %q: %w", name, res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("%w: tag %q of project %s", ErrNotFound, name, p.Name)
		}
		return nil
	})
}

// WorkingCopyStatus compares pixi.toml and pixi.lock in the project
// directory with the installed version, by content hash. A missing
// pixi.lock hashes as empty.
func (s *Service) WorkingCopyStatus(ctx context.Context, projectID uuid.UUID) (WorkingCopyStatus, error) {
	p, err := s.loadProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	wc, err := readWorkingCopy(p.Path)
	if errors.Is(err, ErrWorkingCopyMissing) {
		return WorkingCopyMissing, nil
	}
	if err != nil {
		return "", err
	}
	if p.InstalledVersionID == nil {
		return WorkingCopyNoBase, nil
	}
	installed, err := versionByIDCols(s.db.WithContext(ctx), p.ID, *p.InstalledVersionID)
	if errors.Is(err, ErrNotFound) {
		return WorkingCopyNoBase, nil
	}
	if err != nil {
		return "", err
	}
	if wc.Hash() == installed.ContentHash {
		return WorkingCopyClean, nil
	}
	return WorkingCopyModified, nil
}

// versionByIDCols is versionByID without the file contents.
func versionByIDCols(tx *gorm.DB, projectID, id uuid.UUID) (*ProjectVersion, error) {
	var list []ProjectVersion
	if err := tx.Select(summaryColumns).Where("id = ? AND project_id = ?", id, projectID).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("reading version %s: %w", id, err)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%w: version %s of project %s", ErrNotFound, id, projectID)
	}
	return &list[0], nil
}

// projectIn loads a project inside tx.
func projectIn(tx *gorm.DB, id uuid.UUID) (*Project, error) {
	var p Project
	err := tx.Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: project %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("loading project %s: %w", id, err)
	}
	return &p, nil
}

// summarize builds the summaries of list, versions of p, in order.
func summarize(tx *gorm.DB, p *Project, list []ProjectVersion) ([]ProjectVersionSummary, error) {
	if len(list) == 0 {
		return []ProjectVersionSummary{}, nil
	}
	var maxNumber int
	if err := tx.Model(&ProjectVersion{}).Where("project_id = ?", p.ID).
		Select("COALESCE(MAX(number), 0)").Scan(&maxNumber).Error; err != nil {
		return nil, fmt.Errorf("reading the latest version: %w", err)
	}

	numbers := make(map[uuid.UUID]int, len(list))
	for _, v := range list {
		numbers[v.ID] = v.Number
	}
	var missing []uuid.UUID
	for _, v := range list {
		if v.BaseVersionID != nil {
			if _, ok := numbers[*v.BaseVersionID]; !ok {
				missing = append(missing, *v.BaseVersionID)
			}
		}
	}
	if len(missing) > 0 {
		var bases []ProjectVersion
		if err := tx.Select("id", "number").Where("project_id = ? AND id IN ?", p.ID, missing).Find(&bases).Error; err != nil {
			return nil, fmt.Errorf("reading base versions: %w", err)
		}
		for _, b := range bases {
			numbers[b.ID] = b.Number
		}
	}

	var tags []ProjectTag
	if err := tx.Where("project_id = ?", p.ID).Order("name").Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("reading tags: %w", err)
	}
	byVersion := make(map[uuid.UUID][]string)
	for _, t := range tags {
		byVersion[t.VersionID] = append(byVersion[t.VersionID], t.Name)
	}

	out := make([]ProjectVersionSummary, len(list))
	for i, v := range list {
		names := []string{}
		if v.Number == maxNumber {
			names = append(names, LatestTag)
		}
		names = append(names, byVersion[v.ID]...) // ordered by name
		sum := ProjectVersionSummary{
			ProjectVersion: v,
			Tags:           names,
			Installed:      p.InstalledVersionID != nil && *p.InstalledVersionID == v.ID,
		}
		if v.BaseVersionID != nil {
			if n, ok := numbers[*v.BaseVersionID]; ok {
				sum.BaseNumber = &n
			}
		}
		out[i] = sum
	}
	return out, nil
}

// versionCounts is the version data the project list shows.
type versionCounts struct {
	// Count is the number of versions of the project.
	Count int
	// Latest is the newest version number; nil when there are no versions.
	Latest *int
	// Installed is the number of the version whose ID was passed as
	// installedID; nil when that is nil or not a version of the project.
	Installed *int
}

// versionCounts reports the project's version count, its latest version
// number and the number of the version whose ID is installedID.
func (s *Service) versionCounts(ctx context.Context, projectID uuid.UUID, installedID *uuid.UUID) (versionCounts, error) {
	var row struct {
		Count  int
		Latest *int
	}
	db := s.db.WithContext(ctx)
	if err := db.Model(&ProjectVersion{}).Where("project_id = ?", projectID).
		Select("COUNT(*) AS count, MAX(number) AS latest").Scan(&row).Error; err != nil {
		return versionCounts{}, fmt.Errorf("counting versions: %w", err)
	}
	out := versionCounts{Count: row.Count, Latest: row.Latest}
	if installedID != nil {
		var numbers []int
		if err := db.Model(&ProjectVersion{}).Where("id = ? AND project_id = ?", *installedID, projectID).
			Pluck("number", &numbers).Error; err != nil {
			return versionCounts{}, fmt.Errorf("reading the installed version: %w", err)
		}
		if len(numbers) > 0 {
			out.Installed = &numbers[0]
		}
	}
	return out, nil
}

// deleteProjectVersions deletes every tag and version of the project
// inside tx, the transaction that deletes the project row. It is the only
// code that deletes versions: they are never deleted on their own.
func deleteProjectVersions(tx *gorm.DB, projectID uuid.UUID) error {
	if err := tx.Where("project_id = ?", projectID).Delete(&ProjectTag{}).Error; err != nil {
		return fmt.Errorf("deleting tags: %w", err)
	}
	if err := tx.Where("project_id = ?", projectID).Delete(&ProjectVersion{}).Error; err != nil {
		return fmt.Errorf("deleting versions: %w", err)
	}
	return nil
}

// workingCopy is pixi.toml and pixi.lock as read from a project directory.
type workingCopy struct {
	EnvironmentSpecification
	// HasLock is false when there is no pixi.lock; Lock is then empty.
	HasLock bool
}

// readWorkingCopy reads pixi.toml and pixi.lock from dir. It returns
// ErrWorkingCopyMissing when the directory or pixi.toml is gone.
func readWorkingCopy(dir string) (workingCopy, error) {
	manifest, err := os.ReadFile(filepath.Join(dir, artifact.ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return workingCopy{}, fmt.Errorf("%w: no %s in %s", ErrWorkingCopyMissing, artifact.ManifestFile, dir)
	}
	if err != nil {
		return workingCopy{}, fmt.Errorf("reading %s: %w", artifact.ManifestFile, err)
	}
	wc := workingCopy{EnvironmentSpecification: EnvironmentSpecification{Manifest: string(manifest)}}
	lock, err := os.ReadFile(filepath.Join(dir, artifact.LockFile))
	switch {
	case err == nil:
		wc.Lock, wc.HasLock = string(lock), true
	case !errors.Is(err, fs.ErrNotExist):
		return workingCopy{}, fmt.Errorf("reading %s: %w", artifact.LockFile, err)
	}
	return wc, nil
}
