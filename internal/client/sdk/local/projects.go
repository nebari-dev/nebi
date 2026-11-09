package local

// Owner: U1 (projects). Projects list, create, delete, name validation,
// size on disk, install status, default location.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/utils"
	"gorm.io/gorm"
)

// Project sources. A "managed" project directory was created by Nebi and is
// removed when the project is deleted; a "local" one already existed (the
// user pointed Nebi at it) and is never removed.
const (
	sourceManaged = "managed"
	sourceLocal   = "local"
)

// Job metadata keys of the create job (CONTRACTS.md, job types).
const (
	metaPixiToml    = "pixi_toml"
	metaPath        = "path"
	metaDescription = "description"
	metaTags        = "tags"
	metaInstall     = "install"
)

// createMu serializes the name/path checks and the project insert so two
// concurrent creates cannot take the same name or folder.
var createMu sync.Mutex

// installJobTypes materialize an environment; uninstallJobTypes remove one.
// Legacy env_install/env_uninstall rows still count so projects that predate
// the version model report a sensible status.
var (
	installJobTypes   = []models.JobType{models.JobTypeInstallVersion, models.JobTypeEnvInstall, models.JobTypePullInstall}
	uninstallJobTypes = []models.JobType{models.JobTypeUninstall, models.JobTypeEnvUninstall}
)

// ListProjects returns every local project for the projects list, newest
// first, then runs the registered ProjectDecorators.
func (s *Service) ListProjects(ctx context.Context) ([]shared.ProjectListItem, error) {
	var projects []models.Project
	if err := s.deps.DB.WithContext(ctx).Order("created_at DESC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	items := make([]shared.ProjectListItem, 0, len(projects))
	for i := range projects {
		item, err := s.projectItem(ctx, &projects[i])
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := s.decorateProjects(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetProject returns one project as a list item (decorated).
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (*shared.ProjectListItem, error) {
	p, err := s.loadProject(ctx, id)
	if err != nil {
		return nil, err
	}
	item, err := s.projectItem(ctx, p)
	if err != nil {
		return nil, err
	}
	items := []shared.ProjectListItem{*item}
	if err := s.decorateProjects(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

// ResolveProject finds a project by UUID or by name (CLI arguments).
// Returns shared.ErrNotFound when nothing matches. Implemented by the
// foundation because every unit's CLI commands use it.
func (s *Service) ResolveProject(ctx context.Context, ref string) (*models.Project, error) {
	var p models.Project
	db := s.deps.DB.WithContext(ctx)
	if id, err := uuid.Parse(ref); err == nil {
		if err := db.Where("id = ?", id).First(&p).Error; err == nil {
			return &p, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	err := db.Where("name = ?", ref).Order("created_at DESC").First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("project %q: %w", ref, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CheckName validates a new project name: allowed characters and no
// duplicate among local projects.
func (s *Service) CheckName(ctx context.Context, name string) (shared.NameCheckResult, error) {
	name = strings.TrimSpace(name)
	res := shared.NameCheckResult{Name: name}
	if err := artifact.ValidateProjectName(name); err != nil {
		res.Error = err.Error()
		return res, nil
	}
	taken, err := s.nameTaken(ctx, name)
	if err != nil {
		return res, err
	}
	if taken {
		res.Error = artifact.DuplicateProjectNameError().Message
		return res, nil
	}
	res.Available = true
	return res, nil
}

// CreateProject validates the request (name, at least one platform),
// creates the project row (status pending) and submits a JobTypeCreate job
// with metadata {"pixi_toml", "path", "description", "tags", "install"}.
//
// In the CLI the job runs inline: the result is returned together with the
// job's error when it fails.
func (s *Service) CreateProject(ctx context.Context, req shared.CreateProjectRequest) (*shared.CreateProjectResult, error) {
	name := strings.TrimSpace(req.Name)
	if err := artifact.ValidateProjectName(name); err != nil {
		return nil, toSharedValidation(err)
	}
	pixiToml, err := manifestForCreate(name, req)
	if err != nil {
		return nil, err
	}
	description := strings.TrimSpace(req.Description)
	if err := artifact.ValidateDescription(description); err != nil {
		return nil, toSharedValidation(err)
	}
	tags := cleanTags(req.Tags)
	if err := artifact.ValidateTags(tags); err != nil {
		return nil, toSharedValidation(err)
	}

	createMu.Lock()
	project, err := s.insertPendingProject(ctx, name, req.Path)
	createMu.Unlock()
	if err != nil {
		return nil, err
	}

	job, submitErr := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{
		ProjectID: project.ID,
		Type:      models.JobTypeCreate,
		Metadata: map[string]any{
			metaPixiToml:    pixiToml,
			metaPath:        project.Path,
			metaDescription: description,
			metaTags:        tags,
			metaInstall:     req.Install,
		},
	})
	if job == nil {
		// Nothing ran: drop the row so the name stays free.
		s.deps.DB.WithContext(context.WithoutCancel(ctx)).Unscoped().Delete(&models.Project{}, "id = ?", project.ID)
		if submitErr == nil {
			submitErr = errors.New("job submitter returned no job")
		}
		return nil, submitErr
	}
	// Reload: an inline job (CLI) has already moved the project on.
	p, err := s.loadProjectUnscoped(context.WithoutCancel(ctx), project.ID)
	if err != nil {
		return nil, err
	}
	item, err := s.projectItem(context.WithoutCancel(ctx), p)
	if err != nil {
		return nil, err
	}
	return &shared.CreateProjectResult{
		Project: *item,
		Job:     shared.JobSummaryFrom(*job, p.Name),
	}, submitErr
}

// DeleteProject submits a JobTypeDelete job for the project.
func (s *Service) DeleteProject(ctx context.Context, id uuid.UUID) (*shared.JobSummary, error) {
	p, err := s.loadProject(ctx, id)
	if err != nil {
		return nil, err
	}
	active, err := s.activeJob(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, &shared.ConflictError{
			Code:    "job_active",
			Message: fmt.Sprintf("%s has a job in progress. Wait for it to finish, or cancel it, before deleting the project.", p.Name),
		}
	}
	job, err := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{ProjectID: p.ID, Type: models.JobTypeDelete})
	if job == nil {
		if err == nil {
			err = errors.New("job submitter returned no job")
		}
		return nil, err
	}
	summary := shared.JobSummaryFrom(*job, p.Name)
	return &summary, err
}

// ProjectSize computes the size on disk of the project's folder (pixi.toml,
// pixi.lock and the installed environment under .pixi), stores it on the
// project and returns it. A missing folder is size 0.
func (s *Service) ProjectSize(ctx context.Context, id uuid.UUID) (int64, error) {
	p, err := s.loadProject(ctx, id)
	if err != nil {
		return 0, err
	}
	return s.measure(ctx, p)
}

func (s *Service) measure(ctx context.Context, p *models.Project) (int64, error) {
	dir := s.projectDir(p)
	var size int64
	if _, err := os.Stat(dir); err == nil {
		size, err = utils.GetDirectorySize(dir)
		if err != nil {
			return 0, fmt.Errorf("measuring %s: %w", dir, err)
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	if err := s.deps.DB.WithContext(ctx).Model(&models.Project{}).
		Where("id = ?", p.ID).UpdateColumn("size_bytes", size).Error; err != nil {
		return 0, fmt.Errorf("storing project size: %w", err)
	}
	p.SizeBytes = size
	return size, nil
}

// RunCreateProject is the create job body (client-submitted create jobs
// only): write pixi.toml at the project path, solve the lock, store version
// 1 (description, tags via ApplyTags) and mark the project ready. Sets
// job.VersionNumber = 1. Installing version 1 for "Create and install"
// (metadata install=true) is chained by the job handler in
// internal/client/jobs, which calls the exec SDK job body afterwards, since
// sdk/local cannot import sdk/exec.
func (s *Service) RunCreateProject(ctx context.Context, job *models.Job, log io.Writer) (err error) {
	db := s.deps.DB
	p, err := s.loadProject(ctx, job.ProjectID)
	if err != nil {
		return err
	}
	pixiToml := metaString(job.Metadata, metaPixiToml)
	if strings.TrimSpace(pixiToml) == "" {
		return fmt.Errorf("create job %s has no pixi.toml", job.ID)
	}
	description := metaString(job.Metadata, metaDescription)
	tags := metaStrings(job.Metadata, metaTags)

	defer func() {
		if err == nil {
			return
		}
		// Persist the failure even when ctx was cancelled, and remove the
		// partial folder when Nebi created it.
		bg := context.WithoutCancel(ctx)
		if p.Source == sourceManaged {
			if rmErr := os.RemoveAll(s.projectDir(p)); rmErr != nil {
				fmt.Fprintf(log, "Cleanup of %s failed: %v\n", s.projectDir(p), rmErr)
			} else {
				fmt.Fprintf(log, "Removed partial project folder %s\n", s.projectDir(p))
			}
		}
		db.WithContext(bg).Model(&models.Project{}).Where("id = ?", p.ID).
			Update("status", models.ProjectStatusFailed)
	}()

	if err := db.WithContext(ctx).Model(&models.Project{}).Where("id = ?", p.ID).
		Update("status", models.ProjectStatusCreating).Error; err != nil {
		return err
	}
	if s.deps.Executor == nil {
		return errors.New("no executor configured")
	}
	if err := s.deps.Executor.CreateProject(ctx, p, log, executorCreateOptions(pixiToml)); err != nil {
		return err
	}

	dir := s.projectDir(p)
	manifest, err := os.ReadFile(filepath.Join(dir, artifact.ManifestFile))
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	lock, err := os.ReadFile(filepath.Join(dir, artifact.LockFile))
	if err != nil {
		return fmt.Errorf("reading pixi.lock: %w", err)
	}
	if _, err := artifact.ParseLock(lock); err != nil {
		return fmt.Errorf("reading pixi.lock: %w", err)
	}

	// Same rule as new versions: the connected server's username, else the
	// OS user's name.
	author := s.nvAuthorName(ctx)
	one := 1
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		v := models.ProjectVersion{
			ProjectID:       p.ID,
			VersionNumber:   1,
			ManifestContent: string(manifest),
			LockFileContent: string(lock),
			PackageMetadata: "[]",
			ContentHash:     contenthash.Hash(string(manifest), string(lock)),
			JobID:           &job.ID,
			CreatedBy:       s.deps.LocalUserID,
			AuthorName:      author,
			Description:     description,
		}
		if err := tx.Create(&v).Error; err != nil {
			return fmt.Errorf("storing version 1: %w", err)
		}
		if err := ApplyTags(tx, p.ID, 1, tags, s.deps.LocalUserID); err != nil {
			return err
		}
		return tx.Model(&models.Project{}).Where("id = ?", p.ID).
			Update("status", models.ProjectStatusReady).Error
	})
	if err != nil {
		return err
	}
	job.VersionNumber = &one
	if _, err := s.measure(ctx, p); err != nil {
		fmt.Fprintf(log, "Warning: %v\n", err)
	}
	fmt.Fprintf(log, "Version 1 of %s saved\n", p.Name)
	return nil
}

// RunDeleteProject is the delete job body (client-submitted): uninstall,
// remove the project directory when Nebi created it, and soft-delete the
// project with its versions and remotes.
func (s *Service) RunDeleteProject(ctx context.Context, job *models.Job, log io.Writer) error {
	db := s.deps.DB
	p, err := s.loadProject(ctx, job.ProjectID)
	if err != nil {
		return err
	}
	previous := p.Status
	if err := db.WithContext(ctx).Model(&models.Project{}).Where("id = ?", p.ID).
		Update("status", models.ProjectStatusDeleting).Error; err != nil {
		return err
	}
	restore := func() {
		db.WithContext(context.WithoutCancel(ctx)).Model(&models.Project{}).Where("id = ?", p.ID).
			Update("status", previous)
	}

	if s.deps.Executor != nil {
		if s.deps.Executor.IsEnvInstalled(p) {
			if err := s.deps.Executor.UninstallEnvironment(ctx, p, log); err != nil {
				restore()
				return err
			}
		}
		if p.Source != sourceLocal {
			if err := s.deps.Executor.DeleteProject(ctx, p, log); err != nil {
				restore()
				return err
			}
		} else {
			fmt.Fprintf(log, "Keeping %s: the folder existed before Nebi used it\n", s.projectDir(p))
		}
	}
	if err := ctx.Err(); err != nil {
		restore()
		return err
	}

	// From here on the files are gone: finish the bookkeeping even if the
	// job is cancelled meanwhile.
	bg := context.WithoutCancel(ctx)
	err = db.WithContext(bg).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", p.ID).Delete(&models.ProjectVersion{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", p.ID).Delete(&models.ProjectRemote{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", p.ID).Delete(&models.ProjectTag{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Project{}).Where("id = ?", p.ID).Updates(map[string]any{
			"installed_version": nil,
			"size_bytes":        0,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Project{}, "id = ?", p.ID).Error
	})
	if err != nil {
		return fmt.Errorf("deleting project records: %w", err)
	}
	fmt.Fprintf(log, "Project %s deleted\n", p.Name)
	return nil
}

// ------------------------------------------------------------------ helpers

// projectItem builds the base list item (without decorator fields).
func (s *Service) projectItem(ctx context.Context, p *models.Project) (*shared.ProjectListItem, error) {
	db := s.deps.DB.WithContext(ctx)
	item := &shared.ProjectListItem{
		ID:                   p.ID,
		Name:                 p.Name,
		Path:                 s.projectDir(p),
		Status:               p.Status,
		LastInstalledVersion: p.LastInstalledVersion,
		Remotes:              []shared.RemoteRef{},
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}

	var latest struct{ Max *int }
	if err := db.Model(&models.ProjectVersion{}).Select("MAX(version_number) AS max").
		Where("project_id = ?", p.ID).Scan(&latest).Error; err != nil {
		return nil, fmt.Errorf("latest version of %s: %w", p.Name, err)
	}
	item.LatestVersion = latest.Max

	active, err := s.activeJob(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		id := active.ID
		item.ActiveJobID = &id
	}

	status, err := s.installStatus(ctx, p, active, item.LatestVersion != nil)
	if err != nil {
		return nil, err
	}
	item.InstallStatus = status

	onDisk := p.Status != models.ProjectStatusDeleting && s.deps.Executor != nil && s.deps.Executor.IsEnvInstalled(p)
	if onDisk {
		switch {
		case p.InstalledVersion != nil:
			item.InstalledVersion = p.InstalledVersion
		case p.LastInstalledVersion != nil:
			// Installed before the version model tracked it.
			item.InstalledVersion = p.LastInstalledVersion
		default:
			item.InstalledVersion = item.LatestVersion
		}
		size := p.SizeBytes
		if size == 0 {
			if measured, err := s.measure(ctx, p); err == nil {
				size = measured
			}
		}
		if size > 0 {
			item.SizeBytes = &size
			item.Size = utils.FormatBytes(size)
		}
	}

	var remotes []models.ProjectRemote
	if err := db.Where("project_id = ? AND kind = ?", p.ID, models.RemoteKindRegistry).
		Order("created_at ASC").Find(&remotes).Error; err != nil {
		return nil, fmt.Errorf("remotes of %s: %w", p.Name, err)
	}
	seen := map[string]bool{}
	for _, r := range remotes {
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		ref := shared.RemoteRef{Kind: r.Kind, Name: r.Name}
		if r.RegistryHost != "" {
			ref.URL = r.RegistryHost
			if r.Repository != "" {
				ref.URL = r.RegistryHost + "/" + r.Repository
			}
		}
		item.Remotes = append(item.Remotes, ref)
	}
	return item, nil
}

// installStatus derives the environment status from the active job, the
// latest install/uninstall job and the disk, like the legacy
// service.installStatusFor but for the version-model job types.
func (s *Service) installStatus(ctx context.Context, p *models.Project, active *models.Job, hasVersions bool) (models.InstallStatus, error) {
	if active != nil {
		switch {
		case containsType(installJobTypes, active.Type),
			active.Type == models.JobTypeCreate && metaBool(active.Metadata, metaInstall):
			return models.InstallStatusInstalling, nil
		case containsType(uninstallJobTypes, active.Type):
			return models.InstallStatusUninstalling, nil
		}
	}

	envTypes := append(append([]models.JobType{models.JobTypeCreate}, installJobTypes...), uninstallJobTypes...)
	var jobs []models.Job
	if err := s.deps.DB.WithContext(ctx).
		Where("project_id = ? AND type IN ? AND status IN ?", p.ID, envTypes,
			[]models.JobStatus{models.JobStatusCompleted, models.JobStatusFailed}).
		Order("created_at DESC").Limit(10).Find(&jobs).Error; err != nil {
		return "", fmt.Errorf("install jobs of %s: %w", p.Name, err)
	}
	for _, j := range jobs {
		if j.Type == models.JobTypeCreate && !metaBool(j.Metadata, metaInstall) {
			continue // plain create: no environment involved
		}
		if j.Status == models.JobStatusFailed && j.Type != models.JobTypeEnvUninstall && j.Type != models.JobTypeUninstall {
			// A create-and-install or pull-and-install that failed before
			// any version existed is a create failure, not an install one.
			if (j.Type == models.JobTypeCreate || j.Type == models.JobTypePullInstall) && !hasVersions {
				break
			}
			return models.InstallStatusFailed, nil
		}
		break
	}
	if p.Status != models.ProjectStatusDeleting && s.deps.Executor != nil && s.deps.Executor.IsEnvInstalled(p) {
		return models.InstallStatusInstalled, nil
	}
	return models.InstallStatusNotInstalled, nil
}

// activeJob returns the newest pending or running job of a project.
func (s *Service) activeJob(ctx context.Context, projectID uuid.UUID) (*models.Job, error) {
	var job models.Job
	err := s.deps.DB.WithContext(ctx).
		Where("project_id = ? AND status IN ?", projectID,
			[]models.JobStatus{models.JobStatusPending, models.JobStatusRunning}).
		Order("created_at DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("active job: %w", err)
	}
	return &job, nil
}

func (s *Service) decorateProjects(ctx context.Context, items []shared.ProjectListItem) error {
	if s.deps.Runtime == nil {
		return nil
	}
	for _, d := range s.deps.Runtime.ProjectDecorators() {
		if err := d.DecorateProjects(ctx, items); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) loadProject(ctx context.Context, id uuid.UUID) (*models.Project, error) {
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

// loadProjectUnscoped also finds soft-deleted projects (a create job that
// ran inline and failed still reports its project).
func (s *Service) loadProjectUnscoped(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	var p models.Project
	err := s.deps.DB.WithContext(ctx).Unscoped().Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("project %s: %w", id, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// projectDir is the project's folder on disk.
func (s *Service) projectDir(p *models.Project) string {
	if s.deps.Executor != nil {
		return s.deps.Executor.GetProjectPath(p)
	}
	return p.Path
}

// nameTaken reports whether a local project already uses name (ignoring
// case, like the prototype).
func (s *Service) nameTaken(ctx context.Context, name string) (bool, error) {
	var count int64
	if err := s.deps.DB.WithContext(ctx).Model(&models.Project{}).
		Where("LOWER(name) = LOWER(?)", name).Count(&count).Error; err != nil {
		return false, fmt.Errorf("checking project name: %w", err)
	}
	return count > 0, nil
}

// insertPendingProject checks the name and folder, then stores the project
// row with status pending. Call with createMu held.
func (s *Service) insertPendingProject(ctx context.Context, name, rawPath string) (*models.Project, error) {
	taken, err := s.nameTaken(ctx, name)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, &shared.ConflictError{Code: "name_taken", Message: artifact.DuplicateProjectNameError().Message}
	}
	dir, err := s.resolveProjectPath(name, rawPath)
	if err != nil {
		return nil, err
	}
	source, err := checkProjectFolder(dir)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.deps.DB.WithContext(ctx).Model(&models.Project{}).Where("path = ?", dir).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, &shared.ValidationError{Field: "path", Message: "Another project already uses that folder."}
	}
	p := &models.Project{
		Name:    name,
		OwnerID: s.deps.LocalUserID,
		Status:  models.ProjectStatusPending,
		Source:  source,
		Path:    dir,
	}
	if err := s.deps.DB.WithContext(ctx).Create(p).Error; err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	return p, nil
}

// resolveProjectPath turns the optional Path of a create request into an
// absolute folder: blank means DefaultProjectsDir()/<name>; "~" expands to
// the home directory; relative paths are rejected because the desktop app
// and nebi-web have no meaningful working directory.
func (s *Service) resolveProjectPath(name, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return filepath.Join(s.deps.DefaultProjectsDir(), name), nil
	}
	if raw == "~" || strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", &shared.ValidationError{Field: "path", Message: "Use a full path, e.g. /home/user/projects/my-data-project."}
		}
		raw = filepath.Join(home, raw[1:])
	}
	if !filepath.IsAbs(raw) {
		return "", &shared.ValidationError{Field: "path", Message: "Use a full path, e.g. /home/user/projects/my-data-project."}
	}
	return filepath.Clean(raw), nil
}

// checkProjectFolder decides whether dir can hold a new project and who
// owns it: a missing folder is created by Nebi ("managed"); an existing
// empty folder or one without a pixi workspace is the user's ("local").
func checkProjectFolder(dir string) (string, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return sourceManaged, nil
	}
	if err != nil {
		return "", &shared.ValidationError{Field: "path", Message: fmt.Sprintf("Nebi can’t use that folder: %v", err)}
	}
	if !info.IsDir() {
		return "", &shared.ValidationError{Field: "path", Message: "That path is a file. Choose a folder."}
	}
	for _, f := range []string{artifact.ManifestFile, "pyproject.toml", artifact.LockFile} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return "", &shared.ValidationError{Field: "path", Message: fmt.Sprintf("That folder already has a %s. Choose an empty folder.", f)}
		}
	}
	return sourceLocal, nil
}

// manifestForCreate returns the pixi.toml for version 1 from the form or
// the pixi.toml editor, validated.
func manifestForCreate(name string, req shared.CreateProjectRequest) (string, error) {
	hasToml := strings.TrimSpace(req.PixiToml) != ""
	switch {
	case hasToml && req.Form != nil:
		return "", &shared.ValidationError{Field: artifact.FieldManifest, Message: "Send either the form or a pixi.toml, not both."}
	case req.Form != nil:
		form := *req.Form
		form.Name = name
		form.Channels = cleanTags(form.Channels)
		if len(form.Channels) == 0 {
			form.Channels = []string{artifact.DefaultChannel}
		}
		form.Platforms = cleanTags(form.Platforms)
		requested := make([]shared.RequestedPackage, len(form.Requested))
		for i, p := range form.Requested {
			p.Name = strings.TrimSpace(p.Name)
			p.Constraint = artifact.NormalizeConstraint(p.Constraint)
			requested[i] = p
		}
		form.Requested = requested
		if err := artifact.ValidateForm(form); err != nil {
			return "", toSharedValidation(err)
		}
		return string(artifact.RenderManifest(form)), nil
	case hasToml:
		m, err := artifact.ParseManifest([]byte(req.PixiToml))
		if err != nil {
			return "", &shared.ValidationError{Field: artifact.FieldManifest, Message: fmt.Sprintf("pixi.toml isn’t valid TOML: %v", err)}
		}
		if err := artifact.ValidateManifestStructure(m); err != nil {
			return "", toSharedValidation(err)
		}
		if err := artifact.ValidatePlatforms(m.Form.Platforms); err != nil {
			return "", toSharedValidation(err)
		}
		return req.PixiToml, nil
	default:
		return "", &shared.ValidationError{Field: artifact.FieldPlatforms, Message: "Add at least one platform."}
	}
}

// toSharedValidation converts artifact validation errors (single or a
// list, first one wins) into the SDK's ValidationError.
func toSharedValidation(err error) error {
	var list artifact.ValidationErrors
	if errors.As(err, &list) && len(list) > 0 {
		return &shared.ValidationError{Field: list[0].Field, Message: list[0].Message}
	}
	var one *artifact.ValidationError
	if errors.As(err, &one) {
		return &shared.ValidationError{Field: one.Field, Message: one.Message}
	}
	return &shared.ValidationError{Message: err.Error()}
}

// cleanTags trims, drops blanks and duplicates, keeping order.
func cleanTags(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func containsType(types []models.JobType, t models.JobType) bool {
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

func metaString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func metaBool(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// metaStrings reads a string list that may have been round-tripped through
// JSON ([]any) or set in memory ([]string).
func metaStrings(m map[string]any, key string) []string {
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

func executorCreateOptions(pixiToml string) executor.CreateProjectOptions {
	return executor.CreateProjectOptions{PixiToml: pixiToml}
}
