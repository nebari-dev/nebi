package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pixi"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// ProjectService contains the business logic for project operations.
type ProjectService struct {
	db       *gorm.DB
	queue    queue.Queue
	executor executor.Executor
	rbac     rbac.Provider
	isLocal  bool
	encKey   []byte
	limits   limits.Limits
}

// New creates a new ProjectService.
func New(db *gorm.DB, q queue.Queue, exec executor.Executor, isLocal bool, encKey []byte, rbacProvider rbac.Provider, limitCfg limits.Limits) *ProjectService {
	return &ProjectService{db: db, queue: q, executor: exec, isLocal: isLocal, encKey: encKey, rbac: rbacProvider, limits: limitCfg}
}

// IsLocal reports whether the service is running in local/desktop mode.
func (s *ProjectService) IsLocal() bool { return s.isLocal }

// List returns projects visible to the given user.
// In local mode all projects are returned (no ownership filtering).
func (s *ProjectService) List(userID uuid.UUID) ([]ProjectResponse, error) {
	var projects []models.Project

	if s.isLocal {
		if err := s.db.Preload("Owner").Order("created_at DESC").Find(&projects).Error; err != nil {
			return nil, err
		}
	} else {
		// Team mode: owner + permission-based filtering
		query := s.db.Where("owner_id = ?", userID)

		var permissions []models.Permission
		s.db.Where("user_id = ?", userID).Find(&permissions)

		projectIDs := []uuid.UUID{}
		for _, p := range permissions {
			projectIDs = append(projectIDs, p.ProjectID)
		}

		// Group-mediated permissions: include projects shared with any
		// group the caller belongs to. Casbin grouping rules are the source
		// of truth for membership (same query the matcher uses transitively).
		if userGroups, err := s.rbac.GetUserGroups(userID); err == nil && len(userGroups) > 0 {
			var groupPerms []models.GroupPermission
			s.db.Where("group_id IN ?", userGroups).Find(&groupPerms)
			for _, gp := range groupPerms {
				projectIDs = append(projectIDs, gp.ProjectID)
			}
		}

		if len(projectIDs) > 0 {
			query = query.Or("id IN ?", projectIDs)
		}

		if err := query.Preload("Owner").Order("created_at DESC").Find(&projects).Error; err != nil {
			return nil, err
		}
	}

	result := make([]ProjectResponse, len(projects))
	for i, project := range projects {
		result[i] = NewProjectResponse(project)
		if s.isLocal {
			result[i].InstallStatus = s.installStatusFor(&projects[i])
		}
	}
	return result, nil
}

// Get returns a single project by ID.
func (s *ProjectService) Get(id string) (*ProjectResponse, error) {
	var project models.Project
	if err := s.db.Preload("Owner").Where("id = ?", id).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	resp := NewProjectResponse(project)
	if s.isLocal {
		resp.InstallStatus = s.installStatusFor(&project)
	}
	return &resp, nil
}

// Create validates and creates a new project, queues the creation job,
// grants RBAC owner access, and writes an audit log entry.
func (s *ProjectService) Create(ctx context.Context, req CreateRequest, userID uuid.UUID) (*models.Project, error) {
	if err := s.validateManifestContent("pixi.toml", req.PixiToml); err != nil {
		return nil, err
	}

	// Validate source
	if req.Source != "" && req.Source != "managed" && req.Source != "local" {
		return nil, &ValidationError{Message: "source must be 'managed' or 'local'"}
	}
	if req.Source == "local" && !s.isLocal {
		return nil, &ValidationError{Message: "source 'local' is not allowed in team mode"}
	}
	if req.Source == "local" {
		if req.Path == "" || !filepath.IsAbs(req.Path) {
			return nil, &ValidationError{Message: "local projects require an absolute path"}
		}
	}

	name, err := pixi.ResolveProjectName(req.Name, req.PixiToml)
	if err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("invalid pixi.toml: %v", err)}
	}

	project := models.Project{
		Name:    name,
		OwnerID: userID,
		Status:  models.ProjectStatusPending,
		Source:  req.Source,
		Path:    req.Path,
	}

	var job *models.Job
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Queue creation job
		metadata := map[string]interface{}{}
		if req.PixiToml != "" {
			metadata["pixi_toml"] = req.PixiToml
		}
		if req.ImportStagingDir != "" {
			metadata["import_staging_dir"] = req.ImportStagingDir
		}
		if err := s.validateJobMetadata(metadata); err != nil {
			return err
		}
		if err := s.lockJobAdmission(tx); err != nil {
			return err
		}
		if err := s.checkActiveJobQuotas(tx, userID, uuid.Nil); err != nil {
			return err
		}
		if err := tx.Create(&project).Error; err != nil {
			return fmt.Errorf("create project: %w", err)
		}

		job = &models.Job{
			Type:      models.JobTypeCreate,
			ProjectID: project.ID,
			UserID:    userID,
			Status:    models.JobStatusPending,
			Metadata:  metadata,
		}
		if err := tx.Create(job).Error; err != nil {
			return fmt.Errorf("create job: %w", err)
		}

		audit.LogAction(tx, userID, audit.ActionCreateProject, fmt.Sprintf("project:%s", project.ID.String()), map[string]interface{}{
			"name": project.Name,
		})

		return nil
	})
	if err != nil {
		return nil, s.finishAdmissionError(err)
	}
	if err := s.enqueueAdmittedJob(ctx, job); err != nil {
		_ = s.db.Model(&models.Project{}).Where("id = ?", project.ID).Update("status", models.ProjectStatusFailed).Error
		return nil, err
	}

	// RBAC grant happens outside the transaction because Casbin uses its own
	// DB connection, which would deadlock SQLite inside a transaction.
	if err := s.rbac.GrantProjectAccess(userID, project.ID, "owner"); err != nil {
		return nil, fmt.Errorf("grant owner access: %w", err)
	}

	return &project, nil
}

// Delete queues a deletion job for the project and writes an audit log.
func (s *ProjectService) Delete(ctx context.Context, projectID string, userID uuid.UUID) error {
	var job *models.Job
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.lockJobAdmission(tx); err != nil {
			return err
		}

		var project models.Project
		if err := tx.Where("id = ?", projectID).First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNotFound
			}
			return err
		}
		job = &models.Job{
			Type:      models.JobTypeDelete,
			ProjectID: project.ID,
			UserID:    userID,
			Status:    models.JobStatusPending,
		}
		if err := tx.Create(job).Error; err != nil {
			return fmt.Errorf("create job: %w", err)
		}

		audit.LogAction(tx, userID, audit.ActionDeleteProject, fmt.Sprintf("project:%s", project.ID.String()), map[string]interface{}{
			"name": project.Name,
		})
		return nil
	})
	if err != nil {
		return s.finishAdmissionError(err)
	}

	return s.enqueueAdmittedJob(ctx, job)
}

// GetPixiToml reads the pixi.toml content from the project's filesystem.
func (s *ProjectService) GetPixiToml(projectID string) (string, error) {
	var project models.Project
	if err := s.db.Where("id = ?", projectID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", ErrNotFound
		}
		return "", err
	}

	projectPath := s.executor.GetProjectPath(&project)
	content, err := os.ReadFile(filepath.Join(projectPath, "pixi.toml"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("read pixi.toml: %w", err)
	}
	return string(content), nil
}

// SavePixiToml writes pixi.toml content to the project's filesystem.
func (s *ProjectService) SavePixiToml(projectID string, content string) error {
	var project models.Project
	if err := s.db.Where("id = ?", projectID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}
	if err := s.validateManifestContent("pixi.toml", content); err != nil {
		return err
	}

	projectPath := s.executor.GetProjectPath(&project)
	if err := os.WriteFile(filepath.Join(projectPath, "pixi.toml"), []byte(content), 0644); err != nil {
		return fmt.Errorf("write pixi.toml: %w", err)
	}
	return nil
}

// contentHash computes a deterministic hash of manifest + lock content.
// Returns "sha-" followed by the first 12 hex characters of the SHA-256 digest.
func contentHash(pixiToml, pixiLock string) string {
	return contenthash.Hash(pixiToml, pixiLock)
}

// upsertTag creates or updates a tag for the given project/version.
// If the tag already exists, it updates the version number.
// If it doesn't exist, it creates a new tag record.
func (s *ProjectService) upsertTag(projectID uuid.UUID, tag string, versionNumber int, userID uuid.UUID) error {
	var existing models.ProjectTag
	if err := s.db.Where("project_id = ? AND tag = ?", projectID, tag).First(&existing).Error; err == nil {
		existing.VersionNumber = versionNumber
		return s.db.Save(&existing).Error
	}
	return s.db.Create(&models.ProjectTag{
		ProjectID:     projectID,
		Tag:           tag,
		VersionNumber: versionNumber,
		CreatedBy:     userID,
	}).Error
}

// PushVersion creates a new project version (or deduplicates), writes files,
// handles tags (content hash, latest, optional user tag), and records audit logs.
func (s *ProjectService) PushVersion(ctx context.Context, projectID string, req PushRequest, userID uuid.UUID) (*PushResult, error) {
	var project models.Project
	if err := s.db.Where("id = ?", projectID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.validatePushRequest(req); err != nil {
		return nil, err
	}

	if project.Status != models.ProjectStatusReady {
		return nil, &ValidationError{Message: "Project must be in ready state to push"}
	}

	// Check user-tag conflict before any side effects
	if req.Tag != "" {
		var existingUserTag models.ProjectTag
		if err := s.db.Where("project_id = ? AND tag = ?", project.ID, req.Tag).First(&existingUserTag).Error; err == nil {
			if !req.Force {
				return nil, &ConflictError{
					Message: fmt.Sprintf("tag %q already exists at version %d; use --force to reassign", req.Tag, existingUserTag.VersionNumber),
				}
			}
		}
	}

	// Compute content hash
	hashTag := contentHash(req.PixiToml, req.PixiLock)

	// Check for content deduplication: does a version with this hash already exist?
	var existingHashTag models.ProjectTag
	deduplicated := false
	var versionNumber int

	if err := s.db.Where("project_id = ? AND tag = ?", project.ID, hashTag).First(&existingHashTag).Error; err == nil {
		// Content already exists — deduplicate
		deduplicated = true
		versionNumber = existingHashTag.VersionNumber
	} else {
		// New content — write files and create version
		projectPath := s.executor.GetProjectPath(&project)
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return nil, fmt.Errorf("create project directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(projectPath, "pixi.toml"), []byte(req.PixiToml), 0644); err != nil {
			return nil, fmt.Errorf("write pixi.toml: %w", err)
		}
		if req.PixiLock != "" {
			if err := os.WriteFile(filepath.Join(projectPath, "pixi.lock"), []byte(req.PixiLock), 0644); err != nil {
				return nil, fmt.Errorf("write pixi.lock: %w", err)
			}
		}

		desc := fmt.Sprintf("Pushed %s", project.Name)
		if req.Tag != "" {
			desc = fmt.Sprintf("Pushed as %s:%s", project.Name, req.Tag)
		}

		newVersion := models.ProjectVersion{
			ProjectID:       project.ID,
			ManifestContent: req.PixiToml,
			LockFileContent: req.PixiLock,
			ContentHash:     hashTag,
			PackageMetadata: "[]",
			CreatedBy:       userID,
			Description:     desc,
		}
		if err := s.db.Create(&newVersion).Error; err != nil {
			return nil, fmt.Errorf("create version: %w", err)
		}
		versionNumber = newVersion.VersionNumber

		// Create hash tag
		if err := s.upsertTag(project.ID, hashTag, versionNumber, userID); err != nil {
			return nil, fmt.Errorf("create hash tag: %w", err)
		}
	}

	// Always update "latest" tag
	if err := s.upsertTag(project.ID, "latest", versionNumber, userID); err != nil {
		return nil, fmt.Errorf("update latest tag: %w", err)
	}

	tags := []string{hashTag, "latest"}

	// Handle optional user tag
	if req.Tag != "" {
		if err := s.upsertTag(project.ID, req.Tag, versionNumber, userID); err != nil {
			return nil, fmt.Errorf("create user tag: %w", err)
		}
		tags = append(tags, req.Tag)
	}

	audit.Log(s.db, userID, audit.ActionPush, audit.ResourceProject, project.ID, map[string]interface{}{
		"tags":         tags,
		"version":      versionNumber,
		"content_hash": hashTag,
		"deduplicated": deduplicated,
	})

	return &PushResult{
		VersionNumber: versionNumber,
		Tags:          tags,
		ContentHash:   hashTag,
		Deduplicated:  deduplicated,
		Tag:           req.Tag,
	}, nil
}

// ListVersions returns versions for a project (excluding large file contents).
func (s *ProjectService) ListVersions(projectID string) ([]models.ProjectVersion, error) {
	var versions []models.ProjectVersion
	err := s.db.
		Select("id", "project_id", "version_number", "job_id", "created_by", "description", "created_at").
		Where("project_id = ?", projectID).
		Order("version_number DESC").
		Find(&versions).Error
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// GetVersion returns a specific version by project ID and version number.
func (s *ProjectService) GetVersion(projectID string, versionNum string) (*models.ProjectVersion, error) {
	var version models.ProjectVersion
	err := s.db.
		Where("project_id = ? AND version_number = ?", projectID, versionNum).
		First(&version).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &version, nil
}

// GetVersionFile returns the content of a specific file field from a version.
// field must be "lock" or "manifest".
func (s *ProjectService) GetVersionFile(projectID string, versionNum string, field string) (string, error) {
	var selectField string
	switch field {
	case "lock":
		selectField = "lock_file_content"
	case "manifest":
		selectField = "manifest_content"
	default:
		return "", &ValidationError{Message: "field must be 'lock' or 'manifest'"}
	}

	var version models.ProjectVersion
	err := s.db.
		Select(selectField).
		Where("project_id = ? AND version_number = ?", projectID, versionNum).
		First(&version).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", ErrNotFound
		}
		return "", err
	}

	if field == "lock" {
		return version.LockFileContent, nil
	}
	return version.ManifestContent, nil
}

// ListTags returns tags for a project, ordered by creation time descending.
func (s *ProjectService) ListTags(projectID string) ([]models.ProjectTag, error) {
	var tags []models.ProjectTag
	if err := s.db.Where("project_id = ?", projectID).Order("created_at DESC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}
