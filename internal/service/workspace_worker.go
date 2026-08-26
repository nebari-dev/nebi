package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"github.com/nebari-dev/nebi/internal/process"
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

	if ws.Status.IsTransitional() {
		return nil, &ValidationError{Message: fmt.Sprintf("Workspace is not ready to accept jobs while status is: '%s'", ws.Status)}
	}

	// Verify version exists and belongs to this workspace
	var version models.WorkspaceVersion
	if err := s.db.Where("workspace_id = ? AND version_number = ?", wsID, versionNumber).First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.ValidateVersionContent(ws.PackageManager, version.ManifestContent, version.LockFileContent); err != nil {
		return nil, err
	}

	metadata := map[string]interface{}{
		"version_id":     version.ID.String(),
		"version_number": version.VersionNumber,
	}

	return s.submitJob(ctx, ws.ID.String(), userID, models.JobTypeRollback, metadata, audit.ActionRollbackWorkspace)
}

// CreateVersionSnapshot creates a version snapshot after a successful operation.
// Called by the worker after install, remove, create, and rollback operations.
func (s *WorkspaceService) CreateVersionSnapshot(ctx context.Context, ws *models.Workspace, jobID uuid.UUID, userID uuid.UUID, description string) error {
	envPath := s.executor.GetWorkspacePath(ws)

	manifestContent, err := s.readLimitedTextFile(filepath.Join(envPath, "pixi.toml"), "pixi.toml", s.limits.ManifestBytes)
	if err != nil {
		return fmt.Errorf("failed to read pixi.toml: %w", err)
	}

	lockContent, err := s.readLimitedTextFile(filepath.Join(envPath, "pixi.lock"), "pixi.lock", s.limits.LockBytes)
	if err != nil {
		return fmt.Errorf("failed to read pixi.lock: %w", err)
	}
	if err := s.ValidateVersionContent(ws.PackageManager, manifestContent, lockContent); err != nil {
		return err
	}

	// Get package list from package manager
	pm, err := pkgmgr.NewWithContext(ctx, ws.PackageManager)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	var pkgs []pkgmgr.Package
	pkgs, err = pm.List(ctx, s.listOptions(envPath))
	if err != nil {
		mappedErr := s.mapPackageManagerListError(err)
		var validationErr *ValidationError
		if errors.As(mappedErr, &validationErr) || ctx.Err() != nil || process.IsResourceLimitError(mappedErr) {
			return fmt.Errorf("failed to list packages: %w", mappedErr)
		}
		slog.Warn("Failed to list packages for version snapshot; storing empty package metadata", "workspace_id", ws.ID, "error", mappedErr)
		pkgs = nil
	}

	packageMetadata, err := s.packageMetadataJSON(pkgs)
	if err != nil {
		return err
	}

	createdBy := userID
	if createdBy == uuid.Nil {
		createdBy = ws.OwnerID
	}

	version := models.WorkspaceVersion{
		WorkspaceID:     ws.ID,
		LockFileContent: lockContent,
		ManifestContent: manifestContent,
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

// ResetWorkspaceSize zeroes the stored workspace size (used after the
// installed environment is removed).
func (s *WorkspaceService) ResetWorkspaceSize(wsID uuid.UUID) error {
	return s.db.Model(&models.Workspace{}).Where("id = ?", wsID).Update("size_bytes", 0).Error
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
