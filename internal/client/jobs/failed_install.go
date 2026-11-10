package jobs

// Owner: U8 (jobs). Install-failed linkage: fills
// ProjectListItem.FailedInstallJobID so the "Install failed" badge on the
// projects list and project page links to the failing job.

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

// installJobTypes are the job types that change what is installed. The
// newest finished one decides whether a project shows "Install failed":
// a later successful install or uninstall clears it. create and
// pull_registry count only when their metadata asked to install.
var installJobTypes = []models.JobType{
	models.JobTypeInstallVersion,
	models.JobTypeUninstall,
	models.JobTypePullInstall,
	models.JobTypeCreate,
	models.JobTypePullRegistry,
	// Legacy env jobs (old rows and the legacy API).
	models.JobTypeEnvInstall,
	models.JobTypeEnvUninstall,
}

// IsInstallJob reports whether a job installs or removes an environment.
func IsInstallJob(job *models.Job) bool {
	switch job.Type {
	case models.JobTypeInstallVersion, models.JobTypeUninstall, models.JobTypePullInstall,
		models.JobTypeEnvInstall, models.JobTypeEnvUninstall:
		return true
	case models.JobTypeCreate, models.JobTypePullRegistry:
		install, _ := job.Metadata["install"].(bool)
		return install
	}
	return false
}

// FailedInstallJobs returns, per project, the newest finished install job
// when it failed. Cancelled jobs are ignored: a cancelled install leaves
// the previous state and is not a failure.
func (s *Service) FailedInstallJobs(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	out := map[uuid.UUID]uuid.UUID{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	var jobs []models.Job
	err := s.db(ctx).
		Select("id", "project_id", "type", "status", "metadata", "created_at", "completed_at").
		Where("project_id IN ? AND type IN ? AND status IN ?", projectIDs, installJobTypes,
			[]models.JobStatus{models.JobStatusCompleted, models.JobStatusFailed}).
		Order("COALESCE(completed_at, created_at) DESC, created_at DESC").
		Find(&jobs).Error
	if err != nil {
		return nil, fmt.Errorf("load install jobs: %w", err)
	}
	decided := map[uuid.UUID]bool{}
	for i := range jobs {
		job := &jobs[i]
		if decided[job.ProjectID] || !IsInstallJob(job) {
			continue
		}
		decided[job.ProjectID] = true
		if job.Status == models.JobStatusFailed {
			out[job.ProjectID] = job.ID
		}
	}
	return out, nil
}

// DecorateProjects fills FailedInstallJobID. It never fails the list: on a
// database error the field stays null. The link is only set when sdk/local
// reports install_status=install_failed, so the badge and its link follow
// one rule (a create or pull that failed before any version existed is a
// create failure, not an install failure).
func (s *Service) DecorateProjects(ctx context.Context, items []shared.ProjectListItem) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	failed, err := s.FailedInstallJobs(ctx, ids)
	if err != nil {
		s.opts.Logger.Warn("install-failed linkage skipped", "error", err)
		return nil
	}
	for i := range items {
		if jobID, ok := failed[items[i].ID]; ok && items[i].InstallStatus == models.InstallStatusFailed {
			id := jobID
			items[i].FailedInstallJobID = &id
		} else {
			items[i].FailedInstallJobID = nil
		}
	}
	return nil
}
