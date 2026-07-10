package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
)

// InstallWorkspaceEnv enqueues a job that materializes the workspace
// environment (.pixi/envs) from its lockfile. Local mode only.
func (s *WorkspaceService) InstallWorkspaceEnv(ctx context.Context, wsID string, userID uuid.UUID) (*models.Job, error) {
	return s.submitEnvJob(ctx, wsID, userID, models.JobTypeEnvInstall, audit.ActionInstallEnv)
}

// UninstallWorkspaceEnv enqueues a job that removes the workspace's
// installed environment (.pixi/envs). Local mode only.
func (s *WorkspaceService) UninstallWorkspaceEnv(ctx context.Context, wsID string, userID uuid.UUID) (*models.Job, error) {
	return s.submitEnvJob(ctx, wsID, userID, models.JobTypeEnvUninstall, audit.ActionUninstallEnv)
}

// installStatusFor derives a workspace's install status. An active env
// job wins (installing/uninstalling); otherwise the on-disk environment
// decides (installed); otherwise a failed last install surfaces as
// install_failed; the default is not_installed.
func (s *WorkspaceService) installStatusFor(ws *models.Workspace) models.InstallStatus {
	var latest models.Job
	jobErr := s.db.
		Where("workspace_id = ? AND type IN ?", ws.ID,
			[]models.JobType{models.JobTypeEnvInstall, models.JobTypeEnvUninstall}).
		Order("created_at DESC").
		First(&latest).Error

	if jobErr == nil && (latest.Status == models.JobStatusPending || latest.Status == models.JobStatusRunning) {
		if latest.Type == models.JobTypeEnvInstall {
			return models.InstallStatusInstalling
		}
		return models.InstallStatusUninstalling
	}
	if s.executor.IsEnvInstalled(ws) {
		return models.InstallStatusInstalled
	}
	if jobErr == nil && latest.Type == models.JobTypeEnvInstall && latest.Status == models.JobStatusFailed {
		return models.InstallStatusFailed
	}
	return models.InstallStatusNotInstalled
}

// submitEnvJob validates env-job preconditions shared by install and
// uninstall: local mode only, and at most one env job in flight per
// workspace (prevents double-install and install/uninstall races).
func (s *WorkspaceService) submitEnvJob(ctx context.Context, wsID string, userID uuid.UUID, jobType models.JobType, auditAction string) (*models.Job, error) {
	if !s.isLocal {
		return nil, &ValidationError{Message: "environment install is only available in local mode"}
	}

	var active int64
	err := s.db.Model(&models.Job{}).
		Where("workspace_id = ? AND type IN ? AND status IN ?",
			wsID,
			[]models.JobType{models.JobTypeEnvInstall, models.JobTypeEnvUninstall},
			[]models.JobStatus{models.JobStatusPending, models.JobStatusRunning}).
		Count(&active).Error
	if err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, &ConflictError{Message: "an install or uninstall is already in progress for this workspace"}
	}

	metadata := map[string]interface{}{
		"user_id": userID.String(),
	}
	return s.submitJob(ctx, wsID, userID, jobType, metadata, auditAction)
}
