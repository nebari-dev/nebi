package local

// Owner: U3 (newversion). Create a version from a base version, tag
// semantics, change summary preview.
//
// Product rules (issue #649 and the client prototype):
//   - Every change is a new version. A new version starts from any base
//     version on this machine ("Started from version N"); the base stays
//     as it is.
//   - Creating a version solves pixi.lock for the edited pixi.toml in a
//     staging directory (seeded with the base lock so unchanged packages
//     keep their versions) and stores the result. It never touches the
//     project directory and never installs: the installed version stays
//     installed. "Create and install" is a separate install job.
//   - A draft whose pixi.toml is identical to the base is rejected with
//     ConflictError{Code: "no_changes"} ("Nothing changed in <project>.").
//   - Tags move to the new version ("A tag stays with this version until
//     a newer version uses the same tag"); "latest" always moves to the
//     newest version (ApplyTags).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
)

// Job metadata keys of JobTypeCreateVersion (CONTRACTS.md, job types).
const (
	nvMetaBaseVersion = "base_version"
	nvMetaPixiToml    = "pixi_toml"
	nvMetaDescription = "description"
	nvMetaTags        = "tags"
)

// CodeNoChanges is the ConflictError code returned when a draft does not
// change the base version's pixi.toml.
const CodeNoChanges = "no_changes"

// VersionDraft is the starting point of "New version of <project>": the
// base version's pixi.toml, the same file read into the GUI form, and the
// context the page needs (latest version for the older-base notice, tags
// already used in the project for suggestions).
type VersionDraft struct {
	ProjectID     uuid.UUID `json:"project_id"`
	ProjectName   string    `json:"project_name"`
	BaseVersion   int       `json:"base_version"`
	LatestVersion int       `json:"latest_version"`
	// NewerVersions are the local versions newer than the base, ascending
	// ("Changes made in versions 6 and 7 won't carry over").
	NewerVersions   []int               `json:"newer_versions"`
	PixiToml        string              `json:"pixi_toml"`
	Form            shared.ManifestForm `json:"form"`
	Unrepresentable []string            `json:"unrepresentable"`
	// ExistingTags are the user tags of the project (never content-hash
	// tags), "latest" first.
	ExistingTags []string `json:"existing_tags"`
}

// GetVersionDraft returns the draft for a new version based on version
// base (0 = the latest local version).
func (s *Service) GetVersionDraft(ctx context.Context, projectID uuid.UUID, base int) (*VersionDraft, error) {
	project, err := s.nvLoadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	numbers, err := s.nvVersionNumbers(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(numbers) == 0 {
		return nil, fmt.Errorf("%s has no versions on this machine: %w", project.Name, shared.ErrNotFound)
	}
	latest := numbers[len(numbers)-1]
	if base <= 0 {
		base = latest
	}
	bv, err := s.LoadVersion(ctx, projectID, base)
	if err != nil {
		return nil, err
	}
	m, err := artifact.ParseManifest([]byte(bv.ManifestContent))
	if err != nil {
		return nil, fmt.Errorf("version %d has an unreadable pixi.toml: %w", base, err)
	}
	newer := []int{}
	for _, n := range numbers {
		if n > base {
			newer = append(newer, n)
		}
	}
	tags, err := s.nvProjectTags(ctx, projectID)
	if err != nil {
		return nil, err
	}
	unrep := m.Unrepresentable
	if unrep == nil {
		unrep = []string{}
	}
	return &VersionDraft{
		ProjectID:       project.ID,
		ProjectName:     project.Name,
		BaseVersion:     base,
		LatestVersion:   latest,
		NewerVersions:   newer,
		PixiToml:        bv.ManifestContent,
		Form:            m.Form,
		Unrepresentable: unrep,
		ExistingTags:    tags,
	}, nil
}

// DraftManifest returns the pixi.toml of a draft. With a form, the form is
// written onto the base version's pixi.toml (artifact.ApplyForm), so
// tables and keys the form cannot show survive, the same round-trip rule
// as create project. Otherwise pixiToml is returned as is. The form name
// defaults to the base workspace name (the new version page hides it).
func (s *Service) DraftManifest(ctx context.Context, projectID uuid.UUID, base int, pixiToml string, form *shared.ManifestForm) (string, error) {
	if form == nil {
		return pixiToml, nil
	}
	if strings.TrimSpace(pixiToml) != "" {
		return "", &shared.ValidationError{Field: artifact.FieldManifest, Message: "Send either pixi_toml or form, not both."}
	}
	bv, err := s.LoadVersion(ctx, projectID, base)
	if err != nil {
		return "", s.nvBaseVersionError(base, err)
	}
	f := *form
	// ApplyForm writes dependencies as TOML keys, which would silently merge
	// duplicates; check the form rows first.
	if err := nvValidatePackages(f.Requested, artifact.FieldPackages); err != nil {
		return "", err
	}
	if strings.TrimSpace(f.Name) == "" {
		if bm, perr := artifact.ParseManifest([]byte(bv.ManifestContent)); perr == nil {
			f.Name = bm.Form.Name
		}
	}
	out, err := artifact.ApplyForm([]byte(bv.ManifestContent), f)
	if err != nil {
		return "", &shared.ValidationError{Field: artifact.FieldManifest, Message: err.Error()}
	}
	return string(out), nil
}

// CreateVersion validates the request and submits a JobTypeCreateVersion
// job with metadata {"base_version", "pixi_toml", "description", "tags"}.
func (s *Service) CreateVersion(ctx context.Context, projectID uuid.UUID, req shared.CreateVersionRequest) (*shared.JobSummary, error) {
	project, err := s.nvLoadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if project.Status == models.ProjectStatusDeleting {
		return nil, &shared.ConflictError{Code: "job_active", Message: fmt.Sprintf("%s is being deleted.", project.Name)}
	}
	if req.BaseVersion < 1 {
		return nil, &shared.ValidationError{Field: nvMetaBaseVersion, Message: "Choose the version to start from."}
	}
	base, err := s.LoadVersion(ctx, projectID, req.BaseVersion)
	if err != nil {
		return nil, s.nvBaseVersionError(req.BaseVersion, err)
	}

	tomlMode := req.Form == nil
	if tomlMode && strings.TrimSpace(req.PixiToml) == "" {
		return nil, &shared.ValidationError{Field: artifact.FieldManifest, Message: "pixi.toml needs a [workspace] table."}
	}
	draft, err := s.DraftManifest(ctx, projectID, req.BaseVersion, req.PixiToml, req.Form)
	if err != nil {
		return nil, err
	}
	if _, err := nvValidateDraft(draft, tomlMode); err != nil {
		return nil, err
	}
	if err := artifact.ValidateDescription(req.Description); err != nil {
		return nil, nvValidation(err, artifact.FieldDescription)
	}
	tags, err := NormalizeVersionTags(req.Tags)
	if err != nil {
		return nil, err
	}
	if SameManifest(draft, base.ManifestContent) {
		return nil, &shared.ConflictError{Code: CodeNoChanges, Message: NothingChangedMessage(project.Name)}
	}

	tagsAny := make([]any, len(tags))
	for i, t := range tags {
		tagsAny[i] = t
	}
	job, err := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{
		ProjectID: projectID,
		Type:      models.JobTypeCreateVersion,
		Metadata: map[string]any{
			nvMetaBaseVersion: req.BaseVersion,
			nvMetaPixiToml:    draft,
			nvMetaDescription: strings.TrimSpace(req.Description),
			nvMetaTags:        tagsAny,
		},
	})
	if err != nil {
		return nil, err
	}
	sum := shared.JobSummaryFrom(*job, project.Name)
	return &sum, nil
}

// RunCreateVersion is the create_version job body: solve the lock for the
// edited manifest and store version N+1 (BaseVersion, AuthorName, tags).
// It sets job.VersionNumber to the new number.
func (s *Service) RunCreateVersion(ctx context.Context, job *models.Job, log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	base, ok := nvMetaInt(job.Metadata, nvMetaBaseVersion)
	if !ok || base < 1 {
		return fmt.Errorf("create_version job %s has no base_version", job.ID)
	}
	draft := nvMetaString(job.Metadata, nvMetaPixiToml)
	if strings.TrimSpace(draft) == "" {
		return fmt.Errorf("create_version job %s has no pixi_toml", job.ID)
	}
	description := nvMetaString(job.Metadata, nvMetaDescription)
	tags := nvMetaStrings(job.Metadata, nvMetaTags)

	project, err := s.nvLoadProject(ctx, job.ProjectID)
	if err != nil {
		return err
	}
	bv, err := s.LoadVersion(ctx, project.ID, base)
	if err != nil {
		return s.nvBaseVersionError(base, err)
	}
	if s.deps.Executor == nil {
		return errors.New("no executor configured to solve the environment")
	}

	fmt.Fprintf(log, "Starting a new version of %s from version %d\n", project.Name, base)
	lock, err := s.nvSolveDraft(ctx, project, draft, bv.LockFileContent, log)
	if err != nil {
		return err
	}
	if _, perr := artifact.ParseLock([]byte(lock)); perr != nil {
		fmt.Fprintf(log, "Warning: the new pixi.lock could not be read for package details: %v\n", perr)
	}

	userID := s.deps.LocalUserID
	if userID == uuid.Nil {
		userID = project.OwnerID
	}
	baseRef := base
	jobID := job.ID
	version := models.ProjectVersion{
		ProjectID:       project.ID,
		ManifestContent: draft,
		LockFileContent: lock,
		PackageMetadata: "[]",
		ContentHash:     contenthash.Hash(draft, lock),
		JobID:           &jobID,
		CreatedBy:       userID,
		Description:     description,
		BaseVersion:     &baseRef,
		AuthorName:      s.nvAuthorName(ctx),
	}
	err = s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&version).Error; err != nil {
			return fmt.Errorf("storing the new version: %w", err)
		}
		if err := ApplyTags(tx, project.ID, version.VersionNumber, tags, userID); err != nil {
			return fmt.Errorf("moving tags: %w", err)
		}
		return tx.Model(&models.Project{}).Where("id = ?", project.ID).Update("updated_at", time.Now()).Error
	})
	if err != nil {
		return err
	}
	n := version.VersionNumber
	job.VersionNumber = &n

	summary := []string{fmt.Sprintf("Started from version %d", base)}
	if bm, err := artifact.ParseManifest([]byte(bv.ManifestContent)); err == nil {
		if dm, err := artifact.ParseManifest([]byte(draft)); err == nil {
			summary = append(summary, artifact.SummarizeRequestedChanges(bm.Form.Requested, dm.Form.Requested)...)
		}
	}
	fmt.Fprintf(log, "Version %d created (%s)\n", n, strings.Join(summary, " · "))
	if len(tags) > 0 {
		fmt.Fprintf(log, "Tags moved to version %d: %s\n", n, strings.Join(tags, ", "))
	}
	return nil
}

// nvSolveDraft runs pixi lock for draft in a staging directory seeded with
// the base lock, and returns the new pixi.lock. The project directory (and
// the installed environment in it) is never touched.
func (s *Service) nvSolveDraft(ctx context.Context, project *models.Project, draft, baseLock string, log io.Writer) (string, error) {
	root := s.deps.Executor.StagingRoot()
	if root == "" {
		root = os.TempDir()
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("creating staging directory: %w", err)
	}
	dir, err := os.MkdirTemp(root, "new-version-")
	if err != nil {
		return "", fmt.Errorf("creating staging directory: %w", err)
	}
	defer os.RemoveAll(dir)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	fmt.Fprintf(log, "Writing custom pixi.toml content\n")
	if err := os.WriteFile(filepath.Join(dir, artifact.ManifestFile), []byte(draft), 0o644); err != nil {
		return "", fmt.Errorf("writing pixi.toml: %w", err)
	}
	if baseLock != "" {
		if err := os.WriteFile(filepath.Join(dir, artifact.LockFile), []byte(baseLock), 0o644); err != nil {
			return "", fmt.Errorf("writing base pixi.lock: %w", err)
		}
	}
	staging := &models.Project{ID: project.ID, Name: project.Name, OwnerID: project.OwnerID, Path: dir}
	if err := s.deps.Executor.SolveEnvironment(ctx, staging, log); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("solving the new version: %w", err)
	}
	lock, err := os.ReadFile(filepath.Join(dir, artifact.LockFile))
	if err != nil {
		return "", fmt.Errorf("reading the solved pixi.lock: %w", err)
	}
	if len(strings.TrimSpace(string(lock))) == 0 {
		return "", errors.New("pixi lock wrote an empty pixi.lock")
	}
	return string(lock), nil
}

// PreviewChanges compares a draft pixi.toml with its base version for the
// "Summary of changes" on the new version page. The draft is not solved
// yet, so requested packages are compared by constraint (the prototype's
// rule) and the resolved diff is empty.
func (s *Service) PreviewChanges(ctx context.Context, projectID uuid.UUID, baseVersion int, pixiToml string) (*shared.VersionDiff, error) {
	bv, err := s.LoadVersion(ctx, projectID, baseVersion)
	if err != nil {
		return nil, s.nvBaseVersionError(baseVersion, err)
	}
	if _, err := artifact.ParseManifest([]byte(pixiToml)); err != nil {
		return nil, &shared.ValidationError{Field: artifact.FieldManifest, Message: nvUnreadableToml(err)}
	}
	d, err := artifact.DiffVersions(
		artifact.VersionContent{Toml: []byte(bv.ManifestContent)},
		artifact.VersionContent{Toml: []byte(pixiToml)},
		artifact.DiffOptions{},
	)
	if err != nil {
		return nil, &shared.ValidationError{Field: artifact.FieldManifest, Message: err.Error()}
	}
	if d.Requested == nil {
		d.Requested = []artifact.PackageDiffEntry{}
	}
	d.Resolved = []artifact.PackageDiffEntry{}
	d.ResolvedCounts = artifact.DiffCounts{}
	if d.Channels.Added == nil {
		d.Channels.Added = []string{}
	}
	if d.Channels.Removed == nil {
		d.Channels.Removed = []string{}
	}
	if d.Platforms.Added == nil {
		d.Platforms.Added = []string{}
	}
	if d.Platforms.Removed == nil {
		d.Platforms.Removed = []string{}
	}
	return d, nil
}

// SuggestDescription is the description the new version page pre-fills
// ("Suggested from your changes."): the requested-package changes in the
// prototype's wording, e.g. "Added pandas and rich, removed numpy". Empty
// when no package changed.
func SuggestDescription(baseToml, draftToml string) string {
	bm, err := artifact.ParseManifest([]byte(baseToml))
	if err != nil {
		return ""
	}
	dm, err := artifact.ParseManifest([]byte(draftToml))
	if err != nil {
		return ""
	}
	parts := artifact.SummarizeRequestedChanges(bm.Form.Requested, dm.Form.Requested)
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, ", ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// NothingChangedMessage is the copy for a draft identical to its base.
func NothingChangedMessage(project string) string {
	return fmt.Sprintf("Nothing changed in %s.", project)
}

// SameManifest reports whether two pixi.toml texts are the same content,
// ignoring line endings and trailing whitespace.
func SameManifest(a, b string) bool {
	return nvNormalizeManifest(a) == nvNormalizeManifest(b)
}

func nvNormalizeManifest(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// NormalizeVersionTags trims, de-duplicates and validates the tags of a
// new version. "latest" is dropped (it always moves to the newest version
// on its own) and content-hash tags are reserved.
func NormalizeVersionTags(tags []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		if t == "" || t == LatestTag || seen[t] {
			continue
		}
		if err := artifact.ValidateTag(t); err != nil {
			return nil, nvValidation(err, artifact.FieldTags)
		}
		if IsContentHashTag(t) {
			return nil, &shared.ValidationError{Field: artifact.FieldTags, Message: fmt.Sprintf("“%s” can’t be used: Nebi uses sha- tags for content hashes.", t)}
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, nil
}

// nvValidateDraft checks a draft pixi.toml the way the new version page
// does. In pixi.toml editor mode every problem is reported on the
// "manifest" field (shown under the editor); in form mode on the form
// field it belongs to.
func nvValidateDraft(draft string, tomlMode bool) (*artifact.Manifest, error) {
	field := func(f string) string {
		if tomlMode {
			return artifact.FieldManifest
		}
		return f
	}
	m, err := artifact.ParseManifest([]byte(draft))
	if err != nil {
		return nil, &shared.ValidationError{Field: artifact.FieldManifest, Message: nvUnreadableToml(err)}
	}
	if err := artifact.ValidateManifestStructure(m); err != nil {
		if !tomlMode && m.HasWorkspace && len(m.Form.Platforms) == 0 {
			return nil, &shared.ValidationError{Field: artifact.FieldPlatforms, Message: "Add at least one platform."}
		}
		return nil, nvValidation(err, artifact.FieldManifest)
	}
	if err := artifact.ValidatePlatforms(m.Form.Platforms); err != nil {
		return nil, nvValidation(err, field(artifact.FieldPlatforms))
	}
	if err := artifact.ValidateChannels(m.Form.Channels); err != nil {
		return nil, nvValidation(err, field(artifact.FieldChannels))
	}
	if err := nvValidatePackages(m.Form.Requested, field(artifact.FieldPackages)); err != nil {
		return nil, err
	}
	return m, nil
}

// nvValidatePackages checks package names and duplicates (per source).
func nvValidatePackages(pkgs []artifact.RequestedPackage, field string) error {
	seen := map[string]bool{}
	for _, p := range pkgs {
		if err := artifact.ValidatePackageName(p.Name); err != nil {
			return nvValidation(err, field)
		}
		src := p.Source
		if src == "" {
			src = artifact.SourceConda
		}
		k := string(src) + ":" + strings.ToLower(p.Name)
		if seen[k] {
			return &shared.ValidationError{Field: field, Message: fmt.Sprintf("%s is listed twice.", p.Name)}
		}
		seen[k] = true
	}
	return nil
}

func nvUnreadableToml(err error) string {
	return fmt.Sprintf("pixi.toml can’t be read: %v", err)
}

// nvValidation converts an artifact validation error. When the
// artifact error names no field (or field is forced) the given field is
// used.
func nvValidation(err error, field string) error {
	var ae *artifact.ValidationError
	if errors.As(err, &ae) {
		f := ae.Field
		if f == "" || field == artifact.FieldManifest {
			f = field
		}
		return &shared.ValidationError{Field: f, Message: ae.Message}
	}
	var aes artifact.ValidationErrors
	if errors.As(err, &aes) && len(aes) > 0 {
		return nvValidation(aes[0], field)
	}
	return &shared.ValidationError{Field: field, Message: err.Error()}
}

// nvBaseVersionError explains a base version that is not on this machine.
func (s *Service) nvBaseVersionError(base int, err error) error {
	if errors.Is(err, shared.ErrNotFound) {
		return &shared.ValidationError{Field: nvMetaBaseVersion, Message: fmt.Sprintf("Version %d isn’t on this machine. Pull version %d first.", base, base)}
	}
	return err
}

func (s *Service) nvLoadProject(ctx context.Context, id uuid.UUID) (*models.Project, error) {
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

// nvVersionNumbers lists the project's local version numbers ascending.
func (s *Service) nvVersionNumbers(ctx context.Context, projectID uuid.UUID) ([]int, error) {
	var numbers []int
	err := s.deps.DB.WithContext(ctx).Model(&models.ProjectVersion{}).
		Where("project_id = ?", projectID).
		Order("version_number ASC").
		Pluck("version_number", &numbers).Error
	return numbers, err
}

// nvProjectTags lists the user tags of a project, "latest" first, then by
// name; content-hash tags are left out.
func (s *Service) nvProjectTags(ctx context.Context, projectID uuid.UUID) ([]string, error) {
	var tags []string
	if err := s.deps.DB.WithContext(ctx).Model(&models.ProjectTag{}).
		Where("project_id = ?", projectID).
		Pluck("tag", &tags).Error; err != nil {
		return nil, err
	}
	out := []string{LatestTag}
	var rest []string
	for _, t := range tags {
		if t != LatestTag && !IsContentHashTag(t) {
			rest = append(rest, t)
		}
	}
	sort.Strings(rest)
	return append(out, rest...), nil
}

// nvAuthorName is the display name stored on versions made on this machine:
// the username signed in to the connected server when there is one (the
// version will be pushed there), else the OS user's name.
func (s *Service) nvAuthorName(ctx context.Context) string {
	var creds store.Credentials
	if s.deps.DB.Migrator().HasTable(&store.Credentials{}) {
		if err := s.deps.DB.WithContext(ctx).First(&creds).Error; err == nil && strings.TrimSpace(creds.Username) != "" {
			return strings.TrimSpace(creds.Username)
		}
	}
	if u, err := user.Current(); err == nil {
		if name := strings.TrimSpace(u.Name); name != "" {
			return name
		}
		if u.Username != "" {
			return u.Username
		}
	}
	return "local-user"
}

// nvMetaInt reads an integer from job metadata (int in memory, float64 or
// string after a JSON round trip).
func nvMetaInt(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(v)
		return n, err == nil
	}
	return 0, false
}

func nvMetaString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func nvMetaStrings(m map[string]any, key string) []string {
	switch v := m[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// LatestTag is the tag that always points at the newest version.
const LatestTag = "latest"

// ApplyTags moves tags to version within tx: each tag is upserted in
// project_tags to point at version ("A tag stays with this version until a
// newer version uses the same tag"), and "latest" always moves to the
// highest version number of the project. Every code path that stores a
// version (create project, create version, pull, push on the server side
// mirrors it) must call it. Implemented by the foundation because U1, U3
// and U5 all store versions.
func ApplyTags(tx *gorm.DB, projectID uuid.UUID, version int, tags []string, userID uuid.UUID) error {
	upsert := func(tag string, n int) error {
		var existing models.ProjectTag
		err := tx.Where("project_id = ? AND tag = ?", projectID, tag).First(&existing).Error
		switch {
		case err == nil:
			if existing.VersionNumber == n {
				return nil
			}
			existing.VersionNumber = n
			return tx.Save(&existing).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(&models.ProjectTag{ProjectID: projectID, Tag: tag, VersionNumber: n, CreatedBy: userID}).Error
		default:
			return err
		}
	}
	seen := map[string]bool{}
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" || tag == LatestTag || seen[tag] {
			continue
		}
		seen[tag] = true
		if err := upsert(tag, version); err != nil {
			return fmt.Errorf("tag %q: %w", tag, err)
		}
	}
	var maxVersion struct{ Max *int }
	if err := tx.Model(&models.ProjectVersion{}).
		Select("MAX(version_number) AS max").
		Where("project_id = ?", projectID).
		Scan(&maxVersion).Error; err != nil {
		return err
	}
	latest := version
	if maxVersion.Max != nil && *maxVersion.Max > latest {
		latest = *maxVersion.Max
	}
	return upsert(LatestTag, latest)
}
