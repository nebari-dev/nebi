package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"github.com/nebari-dev/nebi/internal/utils"
	"gorm.io/gorm"
)

// RollbackToVersion creates and enqueues a rollback job.
func (s *WorkspaceService) RollbackToVersion(ctx context.Context, wsID string, versionNumber int, userID uuid.UUID) (*models.Job, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if ws.Status != models.WsStatusReady {
		return nil, &ValidationError{Message: "Workspace is not ready"}
	}

	// Verify version exists and belongs to this workspace
	var version models.WorkspaceVersion
	if err := s.db.Where("workspace_id = ? AND version_number = ?", wsID, versionNumber).First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	metadata := map[string]interface{}{
		"version_id":     version.ID.String(),
		"version_number": version.VersionNumber,
		"user_id":        userID.String(),
	}

	job := &models.Job{
		Type:        models.JobTypeRollback,
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

	audit.LogAction(s.db, userID, "rollback_workspace", fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"version_number": versionNumber,
	})

	return job, nil
}

// CreateVersionSnapshot creates a version snapshot after a successful operation.
// Called by the worker after install, remove, create, and rollback operations.
func (s *WorkspaceService) CreateVersionSnapshot(ctx context.Context, ws *models.Workspace, jobID uuid.UUID, userID uuid.UUID, description string) error {
	envPath := s.executor.GetWorkspacePath(ws)

	manifestContent, err := os.ReadFile(filepath.Join(envPath, "pixi.toml"))
	if err != nil {
		return fmt.Errorf("failed to read pixi.toml: %w", err)
	}

	lockContent, err := os.ReadFile(filepath.Join(envPath, "pixi.lock"))
	if err != nil {
		return fmt.Errorf("failed to read pixi.lock: %w", err)
	}

	// Get package list from package manager
	pm, err := pkgmgr.New(ws.PackageManager)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	pkgs, err := pm.List(ctx, pkgmgr.ListOptions{EnvPath: envPath})
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}

	packageMetadata, err := json.Marshal(pkgs)
	if err != nil {
		return fmt.Errorf("failed to serialize package metadata: %w", err)
	}

	createdBy := userID
	if createdBy == uuid.Nil {
		createdBy = ws.OwnerID
	}

	version := models.WorkspaceVersion{
		WorkspaceID:     ws.ID,
		LockFileContent: string(lockContent),
		ManifestContent: string(manifestContent),
		PackageMetadata: string(packageMetadata),
		JobID:           &jobID,
		CreatedBy:       createdBy,
		Description:     description,
	}

	if err := s.db.Create(&version).Error; err != nil {
		return fmt.Errorf("failed to create version snapshot: %w", err)
	}

	slog.Info("Created version snapshot", "workspace_id", ws.ID, "version_number", version.VersionNumber, "job_id", jobID)
	return nil
}

// UpdateWorkspaceSize calculates and updates the workspace size in the database.
func (s *WorkspaceService) UpdateWorkspaceSize(ws *models.Workspace) {
	envPath := s.executor.GetWorkspacePath(ws)
	sizeBytes, err := utils.GetDirectorySize(envPath)
	if err != nil {
		slog.Warn("Failed to calculate workspace size", "ws_id", ws.ID, "error", err)
		return
	}

	ws.SizeBytes = sizeBytes
	s.db.Save(ws)
	slog.Info("Updated workspace size", "ws_id", ws.ID, "size", utils.FormatBytes(sizeBytes))
}

// SetWorkspaceStatus updates the workspace status in the database.
func (s *WorkspaceService) SetWorkspaceStatus(wsID uuid.UUID, status models.WorkspaceStatus) error {
	return s.db.Model(&models.Workspace{}).Where("id = ?", wsID).Update("status", status).Error
}

// SetWorkspacePath updates the workspace path in the database.
func (s *WorkspaceService) SetWorkspacePath(wsID uuid.UUID, path string) error {
	return s.db.Model(&models.Workspace{}).Where("id = ?", wsID).Update("path", path).Error
}

// SoftDeleteWorkspace soft-deletes a workspace.
func (s *WorkspaceService) SoftDeleteWorkspace(wsID uuid.UUID) error {
	return s.db.Delete(&models.Workspace{}, wsID).Error
}

// GetWorkspacePath returns the filesystem path for a workspace.
func (s *WorkspaceService) GetWorkspacePath(ws *models.Workspace) string {
	return s.executor.GetWorkspacePath(ws)
}
