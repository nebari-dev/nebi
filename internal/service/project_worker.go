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
	"github.com/nebari-dev/nebi/internal/process"
	"github.com/nebari-dev/nebi/internal/utils"
	"gorm.io/gorm"
)

// RollbackToVersion creates and enqueues a rollback job.
func (s *ProjectService) RollbackToVersion(ctx context.Context, projectID string, versionNumber int, userID uuid.UUID) (*models.Job, error) {
	var project models.Project
	if err := s.db.Where("id = ?", projectID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if project.Status.IsTransitional() {
		return nil, &ValidationError{Message: fmt.Sprintf("Project is not ready to accept jobs while status is: '%s'", project.Status)}
	}

	// Verify version exists and belongs to this project
	var version models.ProjectVersion
	if err := s.db.Where("project_id = ? AND version_number = ?", projectID, versionNumber).First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.ValidateVersionContent(version.ManifestContent, version.LockFileContent); err != nil {
		return nil, err
	}

	metadata := map[string]interface{}{
		"version_id":     version.ID.String(),
		"version_number": version.VersionNumber,
	}

	return s.submitJob(ctx, project.ID.String(), userID, models.JobTypeRollback, metadata, audit.ActionRollbackProject)
}

// CreateVersionSnapshot creates a version snapshot after a successful operation.
// Called by the worker after install, remove, create, and rollback operations.
func (s *ProjectService) CreateVersionSnapshot(ctx context.Context, project *models.Project, jobID uuid.UUID, userID uuid.UUID, description string) error {
	envPath := s.executor.GetProjectPath(project)

	manifestContent, err := s.readLimitedTextFile(filepath.Join(envPath, "pixi.toml"), "pixi.toml", s.limits.ManifestBytes)
	if err != nil {
		return fmt.Errorf("failed to read pixi.toml: %w", err)
	}

	lockContent, err := s.readLimitedTextFile(filepath.Join(envPath, "pixi.lock"), "pixi.lock", s.limits.LockBytes)
	if err != nil {
		return fmt.Errorf("failed to read pixi.lock: %w", err)
	}
	if err := s.ValidateVersionContent(manifestContent, lockContent); err != nil {
		return err
	}

	// Get package list from pixi
	pkgs, err := pixiListPackages(ctx, s.listOptions(envPath))
	if err != nil {
		mappedErr := s.mapPixiListError(err)
		var validationErr *ValidationError
		if errors.As(mappedErr, &validationErr) || ctx.Err() != nil || process.IsResourceLimitError(mappedErr) {
			return fmt.Errorf("failed to list packages: %w", mappedErr)
		}
		slog.Warn("Failed to list packages for version snapshot; storing empty package metadata", "project_id", project.ID, "error", mappedErr)
		pkgs = nil
	}

	packageMetadata, err := s.packageMetadataJSON(pkgs)
	if err != nil {
		return err
	}

	createdBy := userID
	if createdBy == uuid.Nil {
		createdBy = project.OwnerID
	}

	version := models.ProjectVersion{
		ProjectID:       project.ID,
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

	slog.Info("Created version snapshot", "project_id", project.ID, "version_number", version.VersionNumber, "job_id", jobID)
	return nil
}

// UpdateProjectSize calculates and updates the project size in the database.
func (s *ProjectService) UpdateProjectSize(project *models.Project) {
	envPath := s.executor.GetProjectPath(project)
	sizeBytes, err := utils.GetDirectorySize(envPath)
	if err != nil {
		slog.Warn("Failed to calculate project size", "project_id", project.ID, "error", err)
		return
	}

	project.SizeBytes = sizeBytes
	s.db.Save(project)
	slog.Info("Updated project size", "project_id", project.ID, "size", utils.FormatBytes(sizeBytes))
}

// ResetProjectSize zeroes the stored project size (used after the
// installed environment is removed).
func (s *ProjectService) ResetProjectSize(projectID uuid.UUID) error {
	return s.db.Model(&models.Project{}).Where("id = ?", projectID).Update("size_bytes", 0).Error
}

// SetProjectStatus updates the project status in the database.
func (s *ProjectService) SetProjectStatus(projectID uuid.UUID, status models.ProjectStatus) error {
	return s.db.Model(&models.Project{}).Where("id = ?", projectID).Update("status", status).Error
}

// SetProjectPath updates the project path in the database.
func (s *ProjectService) SetProjectPath(projectID uuid.UUID, path string) error {
	return s.db.Model(&models.Project{}).Where("id = ?", projectID).Update("path", path).Error
}

// SoftDeleteProject soft-deletes a project.
func (s *ProjectService) SoftDeleteProject(projectID uuid.UUID) error {
	return s.db.Delete(&models.Project{}, projectID).Error
}

// GetProjectPath returns the filesystem path for a project.
func (s *ProjectService) GetProjectPath(project *models.Project) string {
	return s.executor.GetProjectPath(project)
}
