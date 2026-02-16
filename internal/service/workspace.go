package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// WorkspaceService contains the business logic for workspace operations.
type WorkspaceService struct {
	db       *gorm.DB
	queue    queue.Queue
	executor executor.Executor
	isLocal  bool
}

// New creates a new WorkspaceService.
func New(db *gorm.DB, q queue.Queue, exec executor.Executor, isLocal bool) *WorkspaceService {
	return &WorkspaceService{db: db, queue: q, executor: exec, isLocal: isLocal}
}

// List returns workspaces visible to the given user.
// In local mode all workspaces are returned (no ownership filtering).
func (s *WorkspaceService) List(userID uuid.UUID) ([]models.Workspace, error) {
	var workspaces []models.Workspace

	if s.isLocal {
		if err := s.db.Preload("Owner").Order("created_at DESC").Find(&workspaces).Error; err != nil {
			return nil, err
		}
		return workspaces, nil
	}

	// Team mode: owner + permission-based filtering
	query := s.db.Where("owner_id = ?", userID)

	var permissions []models.Permission
	s.db.Where("user_id = ?", userID).Find(&permissions)

	wsIDs := []uuid.UUID{}
	for _, p := range permissions {
		wsIDs = append(wsIDs, p.WorkspaceID)
	}
	if len(wsIDs) > 0 {
		query = query.Or("id IN ?", wsIDs)
	}

	if err := query.Preload("Owner").Order("created_at DESC").Find(&workspaces).Error; err != nil {
		return nil, err
	}
	return workspaces, nil
}

// Get returns a single workspace by ID.
func (s *WorkspaceService) Get(id string) (*models.Workspace, error) {
	var ws models.Workspace
	if err := s.db.Preload("Owner").Where("id = ?", id).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ws, nil
}

// Create validates and creates a new workspace, queues the creation job,
// grants RBAC owner access, and writes an audit log entry.
func (s *WorkspaceService) Create(ctx context.Context, req CreateRequest, userID uuid.UUID) (*models.Workspace, error) {
	// Validate source
	if req.Source != "" && req.Source != "managed" && req.Source != "local" {
		return nil, &ValidationError{Message: "source must be 'managed' or 'local'"}
	}
	if req.Source == "local" && !s.isLocal {
		return nil, &ValidationError{Message: "source 'local' is not allowed in team mode"}
	}
	if req.Source == "local" {
		if req.Path == "" || !filepath.IsAbs(req.Path) {
			return nil, &ValidationError{Message: "local workspaces require an absolute path"}
		}
	}

	packageManager := req.PackageManager
	if packageManager == "" {
		packageManager = "pixi"
	}

	ws := models.Workspace{
		Name:           req.Name,
		OwnerID:        userID,
		Status:         models.WsStatusPending,
		PackageManager: packageManager,
		Source:         req.Source,
		Path:           req.Path,
	}

	if err := s.db.Create(&ws).Error; err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	// Queue creation job
	metadata := map[string]interface{}{}
	if req.PixiToml != "" {
		metadata["pixi_toml"] = req.PixiToml
	}

	job := &models.Job{
		Type:        models.JobTypeCreate,
		WorkspaceID: ws.ID,
		Status:      models.JobStatusPending,
		Metadata:    metadata,
	}
	if err := s.db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		return nil, fmt.Errorf("enqueue job: %w", err)
	}

	// Grant owner access
	if err := rbac.GrantWorkspaceAccess(userID, ws.ID, "owner"); err != nil {
		return nil, fmt.Errorf("grant owner access: %w", err)
	}

	// Audit
	audit.LogAction(s.db, userID, audit.ActionCreateWorkspace, fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"name":            ws.Name,
		"package_manager": ws.PackageManager,
	})

	return &ws, nil
}

// Delete queues a deletion job for the workspace and writes an audit log.
func (s *WorkspaceService) Delete(ctx context.Context, wsID string, userID uuid.UUID) error {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	job := &models.Job{
		Type:        models.JobTypeDelete,
		WorkspaceID: ws.ID,
		Status:      models.JobStatusPending,
	}
	if err := s.db.Create(job).Error; err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		return fmt.Errorf("enqueue job: %w", err)
	}

	audit.LogAction(s.db, userID, audit.ActionDeleteWorkspace, fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"name": ws.Name,
	})

	return nil
}

// GetPixiToml reads the pixi.toml content from the workspace's filesystem.
func (s *WorkspaceService) GetPixiToml(wsID string) (string, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", ErrNotFound
		}
		return "", err
	}

	wsPath := s.executor.GetWorkspacePath(&ws)
	content, err := os.ReadFile(filepath.Join(wsPath, "pixi.toml"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("read pixi.toml: %w", err)
	}
	return string(content), nil
}

// SavePixiToml writes pixi.toml content to the workspace's filesystem.
func (s *WorkspaceService) SavePixiToml(wsID string, content string) error {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	wsPath := s.executor.GetWorkspacePath(&ws)
	if err := os.WriteFile(filepath.Join(wsPath, "pixi.toml"), []byte(content), 0644); err != nil {
		return fmt.Errorf("write pixi.toml: %w", err)
	}
	return nil
}

// contentHash computes a deterministic hash of manifest + lock content.
// Returns "sha-" followed by the first 12 hex characters of the SHA-256 digest.
func contentHash(pixiToml, pixiLock string) string {
	h := sha256.New()
	h.Write([]byte(pixiToml))
	h.Write([]byte("\n---\n"))
	h.Write([]byte(pixiLock))
	return fmt.Sprintf("sha-%x", h.Sum(nil)[:6]) // 6 bytes = 12 hex chars
}

// upsertTag creates or updates a tag for the given workspace/version.
// If the tag already exists, it updates the version number.
// If it doesn't exist, it creates a new tag record.
func (s *WorkspaceService) upsertTag(wsID uuid.UUID, tag string, versionNumber int, userID uuid.UUID) error {
	var existing models.WorkspaceTag
	if err := s.db.Where("workspace_id = ? AND tag = ?", wsID, tag).First(&existing).Error; err == nil {
		existing.VersionNumber = versionNumber
		return s.db.Save(&existing).Error
	}
	return s.db.Create(&models.WorkspaceTag{
		WorkspaceID:   wsID,
		Tag:           tag,
		VersionNumber: versionNumber,
		CreatedBy:     userID,
	}).Error
}

// PushVersion creates a new workspace version (or deduplicates), writes files,
// handles tags (content hash, latest, optional user tag), and records audit logs.
func (s *WorkspaceService) PushVersion(ctx context.Context, wsID string, req PushRequest, userID uuid.UUID) (*PushResult, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if ws.Status != models.WsStatusReady {
		return nil, &ValidationError{Message: "Workspace must be in ready state to push"}
	}

	// Check user-tag conflict before any side effects
	if req.Tag != "" {
		var existingUserTag models.WorkspaceTag
		if err := s.db.Where("workspace_id = ? AND tag = ?", ws.ID, req.Tag).First(&existingUserTag).Error; err == nil {
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
	var existingHashTag models.WorkspaceTag
	deduplicated := false
	var versionNumber int

	if err := s.db.Where("workspace_id = ? AND tag = ?", ws.ID, hashTag).First(&existingHashTag).Error; err == nil {
		// Content already exists — deduplicate
		deduplicated = true
		versionNumber = existingHashTag.VersionNumber
	} else {
		// New content — write files and create version
		wsPath := s.executor.GetWorkspacePath(&ws)
		if err := os.MkdirAll(wsPath, 0755); err != nil {
			return nil, fmt.Errorf("create workspace directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(wsPath, "pixi.toml"), []byte(req.PixiToml), 0644); err != nil {
			return nil, fmt.Errorf("write pixi.toml: %w", err)
		}
		if req.PixiLock != "" {
			if err := os.WriteFile(filepath.Join(wsPath, "pixi.lock"), []byte(req.PixiLock), 0644); err != nil {
				return nil, fmt.Errorf("write pixi.lock: %w", err)
			}
		}

		desc := fmt.Sprintf("Pushed %s", ws.Name)
		if req.Tag != "" {
			desc = fmt.Sprintf("Pushed as %s:%s", ws.Name, req.Tag)
		}

		newVersion := models.WorkspaceVersion{
			WorkspaceID:     ws.ID,
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
		if err := s.upsertTag(ws.ID, hashTag, versionNumber, userID); err != nil {
			return nil, fmt.Errorf("create hash tag: %w", err)
		}
	}

	// Always update "latest" tag
	if err := s.upsertTag(ws.ID, "latest", versionNumber, userID); err != nil {
		return nil, fmt.Errorf("update latest tag: %w", err)
	}

	tags := []string{hashTag, "latest"}

	// Handle optional user tag
	if req.Tag != "" {
		if err := s.upsertTag(ws.ID, req.Tag, versionNumber, userID); err != nil {
			return nil, fmt.Errorf("create user tag: %w", err)
		}
		tags = append(tags, req.Tag)
	}

	audit.Log(s.db, userID, audit.ActionPush, audit.ResourceWorkspace, ws.ID, map[string]interface{}{
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

// ListVersions returns versions for a workspace (excluding large file contents).
func (s *WorkspaceService) ListVersions(wsID string) ([]models.WorkspaceVersion, error) {
	var versions []models.WorkspaceVersion
	err := s.db.
		Select("id", "workspace_id", "version_number", "job_id", "created_by", "description", "created_at").
		Where("workspace_id = ?", wsID).
		Order("version_number DESC").
		Find(&versions).Error
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// GetVersion returns a specific version by workspace ID and version number.
func (s *WorkspaceService) GetVersion(wsID string, versionNum string) (*models.WorkspaceVersion, error) {
	var version models.WorkspaceVersion
	err := s.db.
		Where("workspace_id = ? AND version_number = ?", wsID, versionNum).
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
func (s *WorkspaceService) GetVersionFile(wsID string, versionNum string, field string) (string, error) {
	var selectField string
	switch field {
	case "lock":
		selectField = "lock_file_content"
	case "manifest":
		selectField = "manifest_content"
	default:
		return "", &ValidationError{Message: "field must be 'lock' or 'manifest'"}
	}

	var version models.WorkspaceVersion
	err := s.db.
		Select(selectField).
		Where("workspace_id = ? AND version_number = ?", wsID, versionNum).
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

// ListTags returns tags for a workspace, ordered by creation time descending.
func (s *WorkspaceService) ListTags(wsID string) ([]models.WorkspaceTag, error) {
	var tags []models.WorkspaceTag
	if err := s.db.Where("workspace_id = ?", wsID).Order("created_at DESC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}
