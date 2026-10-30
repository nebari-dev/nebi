package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

type lockedJobValidation func(tx *gorm.DB, project *models.Project) error

// submitJob validates the project is ready, creates a Job record, enqueues it,
// and writes an audit log. This is the common pattern for all async operations.
func (s *ProjectService) submitJob(ctx context.Context, projectID string, userID uuid.UUID,
	jobType models.JobType, metadata map[string]interface{}, auditAction string, lockedValidations ...lockedJobValidation) (*models.Job, error) {

	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if err := s.validateJobMetadata(metadata); err != nil {
		return nil, err
	}

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

		if project.Status.IsTransitional() {
			return &ValidationError{Message: fmt.Sprintf("Project is not ready to accept jobs while status is: '%s'", project.Status)}
		}
		for _, validate := range lockedValidations {
			if err := validate(tx, &project); err != nil {
				return err
			}
		}
		if err := s.checkActiveJobQuotas(tx, userID, project.ID); err != nil {
			return err
		}

		job = &models.Job{
			Type:      jobType,
			ProjectID: project.ID,
			UserID:    userID,
			Status:    models.JobStatusPending,
			Metadata:  metadata,
		}
		if err := tx.Create(job).Error; err != nil {
			return fmt.Errorf("create job: %w", err)
		}

		audit.LogAction(tx, userID, auditAction, fmt.Sprintf("project:%s", project.ID.String()), metadata)
		return nil
	})
	if err != nil {
		return nil, s.finishAdmissionError(err)
	}
	if err := s.enqueueAdmittedJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *ProjectService) enqueueAdmittedJob(ctx context.Context, job *models.Job) error {
	if err := s.queue.Enqueue(ctx, job); err != nil {
		now := time.Now()
		_ = s.db.Model(&models.Job{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"status":       models.JobStatusFailed,
			"error":        fmt.Sprintf("enqueue job: %v", err),
			"completed_at": now,
		}).Error
		return fmt.Errorf("enqueue job: %w", err)
	}
	return nil
}

// InstallPackages creates and enqueues an install-packages job.
func (s *ProjectService) InstallPackages(ctx context.Context, projectID string, packages []string, userID uuid.UUID) (*models.Job, error) {
	if err := s.validatePackages(packages); err != nil {
		return nil, err
	}
	metadata := map[string]interface{}{
		"packages": packages,
	}
	return s.submitJob(ctx, projectID, userID, models.JobTypeInstall, metadata, audit.ActionInstallPackage, s.validateProjectManifestAndLockForJob)
}

// SolveProject creates and enqueues a solve job (pixi install from current pixi.toml).
func (s *ProjectService) SolveProject(ctx context.Context, projectID string, userID uuid.UUID) (*models.Job, error) {
	return s.submitJob(ctx, projectID, userID, models.JobTypeUpdate, nil, audit.ActionSolveProject, s.validateProjectManifestForJob)
}

// RemovePackage creates and enqueues a remove-package job.
func (s *ProjectService) RemovePackage(ctx context.Context, projectID string, packageName string, userID uuid.UUID) (*models.Job, error) {
	if err := s.validatePackages([]string{packageName}); err != nil {
		return nil, err
	}
	metadata := map[string]interface{}{
		"packages": []string{packageName},
	}
	return s.submitJob(ctx, projectID, userID, models.JobTypeRemove, metadata, audit.ActionRemovePackage, s.validateProjectManifestAndLockForJob)
}

// ListPackages returns packages for a project, auto-syncing from disk for local projects with no DB records.
func (s *ProjectService) ListPackages(projectID string) ([]models.Package, error) {
	var project models.Project
	if err := s.db.Where("id = ?", projectID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var packages []models.Package
	if err := s.db.Where("project_id = ?", project.ID).Find(&packages).Error; err != nil {
		return nil, fmt.Errorf("fetch packages: %w", err)
	}

	// Auto-sync: if local project has 0 packages in DB, populate from disk
	if len(packages) == 0 && project.Source == "local" && project.Status == models.ProjectStatusReady {
		if synced := s.syncPackagesFromDisk(&project); synced != nil {
			packages = synced
		}
	}

	return packages, nil
}

// syncPackagesFromDisk runs the package manager list and populates the DB for a local project.
func (s *ProjectService) syncPackagesFromDisk(project *models.Project) []models.Package {
	projectPath := s.executor.GetProjectPath(project)

	listed, err := pixiListPackages(context.Background(), s.listOptions(projectPath))
	if err != nil {
		slog.Warn("syncPackagesFromDisk: failed to list packages", "error", s.mapPixiListError(err), "path", projectPath)
		return nil
	}
	if err := s.validateListedPackages("package list", listed); err != nil {
		slog.Warn("syncPackagesFromDisk: package list exceeds limits", "error", err, "path", projectPath)
		return nil
	}

	var result []models.Package
	for _, p := range listed {
		pkg := models.Package{
			ProjectID: project.ID,
			Name:      p.Name,
			Version:   p.Version,
		}
		if err := s.db.Create(&pkg).Error; err != nil {
			slog.Warn("syncPackagesFromDisk: failed to save package", "error", err, "name", p.Name)
			continue
		}
		result = append(result, pkg)
	}

	return result
}

// SyncPackagesFromProject lists packages from the project on disk and saves them to the DB.
// Called by the worker after install/remove/create/rollback operations.
func (s *ProjectService) SyncPackagesFromProject(ctx context.Context, project *models.Project) error {
	projectPath := s.executor.GetProjectPath(project)

	pkgs, err := pixiListPackages(ctx, s.listOptions(projectPath))
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", s.mapPixiListError(err))
	}
	if err := s.validateListedPackages("package list", pkgs); err != nil {
		return err
	}

	// Clear existing packages
	s.db.Where("project_id = ?", project.ID).Delete(&models.Package{})

	// Insert new packages
	for _, pkg := range pkgs {
		dbPkg := models.Package{
			ProjectID: project.ID,
			Name:      pkg.Name,
			Version:   pkg.Version,
		}
		if err := s.db.Create(&dbPkg).Error; err != nil {
			slog.Error("Failed to save package", "package", pkg.Name, "error", err)
		}
	}

	return nil
}

// SaveInstalledPackages records newly installed packages in the DB.
func (s *ProjectService) SaveInstalledPackages(projectID uuid.UUID, packages []string) {
	for _, pkgName := range packages {
		pkg := models.Package{
			ProjectID: projectID,
			Name:      pkgName,
		}
		s.db.Create(&pkg)
	}
}

// DeletePackagesByName removes specific packages from the DB.
func (s *ProjectService) DeletePackagesByName(projectID uuid.UUID, packages []string) {
	for _, pkgName := range packages {
		s.db.Where("project_id = ? AND name = ?", projectID, pkgName).Delete(&models.Package{})
	}
}

// DeleteAllPackages removes all packages for a project from the DB.
func (s *ProjectService) DeleteAllPackages(projectID uuid.UUID) {
	s.db.Where("project_id = ?", projectID).Delete(&models.Package{})
}
