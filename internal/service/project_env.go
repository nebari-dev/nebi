package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// InstallProjectEnv enqueues a job that materializes the project
// environment (.pixi/envs) from its lockfile. Local mode only.
func (s *ProjectService) InstallProjectEnv(ctx context.Context, projectID string, userID uuid.UUID) (*models.Job, error) {
	return s.submitEnvJob(ctx, projectID, userID, models.JobTypeEnvInstall, audit.ActionInstallEnv)
}

// UninstallProjectEnv enqueues a job that removes the project's
// installed environment (.pixi/envs). Local mode only.
func (s *ProjectService) UninstallProjectEnv(ctx context.Context, projectID string, userID uuid.UUID) (*models.Job, error) {
	return s.submitEnvJob(ctx, projectID, userID, models.JobTypeEnvUninstall, audit.ActionUninstallEnv)
}

// installStatusFor derives a project's install status. An active env
// job wins (installing/uninstalling); otherwise a failed last install
// wins over stale on-disk state (pixi install can leave a partial
// .pixi/envs behind before failing); otherwise the on-disk environment
// decides (installed); the default is not_installed.
func (s *ProjectService) installStatusFor(project *models.Project) models.InstallStatus {
	var latest models.Job
	jobErr := s.db.
		Where("project_id = ? AND type IN ?", project.ID,
			[]models.JobType{models.JobTypeEnvInstall, models.JobTypeEnvUninstall}).
		Order("created_at DESC").
		First(&latest).Error

	if jobErr == nil && (latest.Status == models.JobStatusPending || latest.Status == models.JobStatusRunning) {
		if latest.Type == models.JobTypeEnvInstall {
			return models.InstallStatusInstalling
		}
		return models.InstallStatusUninstalling
	}
	if jobErr == nil && latest.Type == models.JobTypeEnvInstall && latest.Status == models.JobStatusFailed {
		return models.InstallStatusFailed
	}
	if s.executor.IsEnvInstalled(project) {
		return models.InstallStatusInstalled
	}
	return models.InstallStatusNotInstalled
}

// submitEnvJob validates env-job preconditions shared by install and
// uninstall: local mode only, and at most one env job in flight per
// project (prevents double-install and install/uninstall races).
func (s *ProjectService) submitEnvJob(ctx context.Context, projectID string, userID uuid.UUID, jobType models.JobType, auditAction string) (*models.Job, error) {
	if !s.isLocal {
		return nil, &ValidationError{Message: "environment install is only available in local mode"}
	}

	validations := []lockedJobValidation{func(tx *gorm.DB, project *models.Project) error {
		var active int64
		err := tx.Model(&models.Job{}).
			Where("project_id = ? AND type IN ? AND status IN ?",
				project.ID,
				[]models.JobType{models.JobTypeEnvInstall, models.JobTypeEnvUninstall},
				[]models.JobStatus{models.JobStatusPending, models.JobStatusRunning}).
			Count(&active).Error
		if err != nil {
			return err
		}
		if active > 0 {
			return &ConflictError{Message: "an install or uninstall is already in progress for this project"}
		}
		return nil
	}}
	if jobType == models.JobTypeEnvInstall {
		validations = append(validations, s.validateProjectManifestAndLockForJob)
	}
	return s.submitJob(ctx, projectID, userID, jobType, nil, auditAction, validations...)
}
