package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	resourcemetrics "github.com/nebari-dev/nebi/internal/metrics"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// JobService contains business logic for job operations.
type JobService struct {
	db      *gorm.DB
	isLocal bool
}

// NewJobService creates a new JobService. In local mode job visibility is
// not restricted by project ownership: the whole machine belongs to one
// person and every request runs as the synthetic local-user, so ownership
// filtering would hide jobs for projects created under a different mode.
func NewJobService(db *gorm.DB, isLocal bool) *JobService {
	return &JobService{db: db, isLocal: isLocal}
}

// RecoverInterruptedJobs settles work abandoned by the previous process. Call
// only at startup, before starting the worker or accepting requests. Each Nebi
// instance must own its database; an in-memory queue cannot share job ownership.
func (s *JobService) RecoverInterruptedJobs(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Job{}).
			Where("status IN ?", activeJobStatuses).
			Updates(map[string]interface{}{
				"status":       models.JobStatusFailed,
				"error":        "Job interrupted by application shutdown; retry the operation",
				"completed_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("recover interrupted jobs: %w", err)
		}
		// A crash may occur between updating a project and its job row, so
		// also settle transitional projects whose job is already terminal.
		if err := tx.Model(&models.Project{}).
			Where("status IN ?", []models.ProjectStatus{
				models.ProjectStatusPending, models.ProjectStatusCreating, models.ProjectStatusDeleting,
			}).Update("status", models.ProjectStatusFailed).Error; err != nil {
			return fmt.Errorf("recover interrupted projects: %w", err)
		}
		return nil
	})
}

// ListJobs returns jobs for projects owned by the given user, or all
// jobs in local mode.
func (s *JobService) ListJobs(userID uuid.UUID) ([]models.Job, error) {
	var jobs []models.Job
	query := s.db.
		Select("jobs.*").
		Joins("JOIN projects ON projects.id = jobs.project_id")
	if !s.isLocal {
		query = query.Where("projects.owner_id = ?", userID)
	}
	err := query.Order("jobs.created_at DESC").Find(&jobs).Error

	if err != nil {
		return nil, fmt.Errorf("fetch jobs: %w", err)
	}
	return jobs, nil
}

// GetJob returns a single job by ID. Outside local mode it verifies the
// user owns the project.
func (s *JobService) GetJob(jobID string, userID uuid.UUID) (*models.Job, error) {
	var job models.Job
	query := s.db.
		Select("jobs.*").
		Joins("JOIN projects ON projects.id = jobs.project_id").
		Where("jobs.id = ?", jobID)
	if !s.isLocal {
		query = query.Where("projects.owner_id = ?", userID)
	}
	err := query.First(&job).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch job: %w", err)
	}
	return &job, nil
}

// GetJobForStreaming returns a job by ID for SSE streaming. Outside local
// mode it verifies the user owns the project. Returns the job regardless
// of status (caller decides what to do with completed jobs).
func (s *JobService) GetJobForStreaming(jobID uuid.UUID, userID uuid.UUID) (*models.Job, error) {
	var job models.Job
	query := s.db.
		Select("jobs.*").
		Joins("JOIN projects ON projects.id = jobs.project_id").
		Where("jobs.id = ?", jobID)
	if !s.isLocal {
		query = query.Where("projects.owner_id = ?", userID)
	}
	err := query.First(&job).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch job: %w", err)
	}
	return &job, nil
}

// --- Worker-facing methods ---

// MarkRunning sets a job's status to running with a start timestamp.
func (s *JobService) MarkRunning(job *models.Job) {
	job.Status = models.JobStatusRunning
	now := time.Now()
	job.StartedAt = &now
	s.db.Save(job)
}

// StartJob moves a job to running unless it was cancelled while it waited
// in the queue, and reports whether the job should run. The status change
// is conditional so a cancel that lands between the queue and the start is
// never overwritten. Rows in any other state (e.g. a job object that was
// never persisted as pending) fall back to MarkRunning.
func (s *JobService) StartJob(job *models.Job) bool {
	now := time.Now()
	res := s.db.Model(&models.Job{}).
		Where("id = ? AND status = ?", job.ID, models.JobStatusPending).
		Updates(map[string]interface{}{"status": models.JobStatusRunning, "started_at": now})
	if res.Error == nil && res.RowsAffected == 1 {
		job.Status = models.JobStatusRunning
		job.StartedAt = &now
		return true
	}
	if s.IsCancelled(job.ID) {
		return false
	}
	s.MarkRunning(job)
	return true
}

// CancelActive marks a pending or running job cancelled in the database
// and reports whether a row changed. A pending job is then skipped by the
// worker; a running job is stopped by whichever process runs it (the
// worker and the CLI inline runner watch their job's row). Finished jobs
// are left as they are.
func (s *JobService) CancelActive(jobID uuid.UUID) (bool, error) {
	res := s.db.Model(&models.Job{}).
		Where("id = ? AND status IN ?", jobID, activeJobStatuses).
		Updates(map[string]interface{}{
			"status":       models.JobStatusCancelled,
			"error":        "Cancelled",
			"completed_at": time.Now(),
		})
	if res.Error != nil {
		return false, fmt.Errorf("cancel job: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// MarkCompleted sets a job's status to completed with final logs.
func (s *JobService) MarkCompleted(job *models.Job, logs string) {
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Status = models.JobStatusCompleted
	job.Logs = logs
	s.db.Save(job)
}

// MarkFailed sets a job's status to failed with error and final logs.
func (s *JobService) MarkFailed(job *models.Job, logs string, errMsg string) {
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Status = models.JobStatusFailed
	job.Error = errMsg
	job.Logs = logs
	s.db.Save(job)
}

// MarkCancelled sets a job's status to cancelled with final logs. Used when a
// user cancels a running job (issue #649) or a pending one before it starts.
func (s *JobService) MarkCancelled(job *models.Job, logs string) {
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Status = models.JobStatusCancelled
	job.Error = "Cancelled"
	job.Logs = logs
	s.db.Save(job)
}

// IsCancelled reports whether the job row was already marked cancelled,
// e.g. a pending job cancelled while it waited in the queue.
func (s *JobService) IsCancelled(jobID uuid.UUID) bool {
	var status models.JobStatus
	if err := s.db.Model(&models.Job{}).Where("id = ?", jobID).Select("status").Scan(&status).Error; err != nil {
		return false
	}
	return status == models.JobStatusCancelled
}

// MarkPanicked sets a job as failed after a panic recovery.
func (s *JobService) MarkPanicked(job *models.Job, panicMsg string) {
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Status = models.JobStatusFailed
	job.Error = panicMsg
	s.db.Save(job)
}

// RecordFailedEnvInstall writes an already-failed env-install job for a
// project whose environment was reinstalled outside the explicit
// install flow (e.g. the worker's auto-reinstall after an update or
// rollback). It exists so that failure surfaces through the same
// install_status derivation an explicit `nebi project install` failure
// would, without ever failing the job that triggered the reinstall.
func (s *JobService) RecordFailedEnvInstall(projectID uuid.UUID, errMsg string) error {
	now := time.Now()
	job := &models.Job{
		ProjectID:   projectID,
		Type:        models.JobTypeEnvInstall,
		Status:      models.JobStatusFailed,
		Error:       errMsg,
		CompletedAt: &now,
	}
	return s.db.Create(job).Error
}

func (s *JobService) RecordJobTimeout() error {
	return resourcemetrics.IncJobTimeout(s.db)
}

// FlushLogs persists the current log content for a job.
func (s *JobService) FlushLogs(jobID uuid.UUID, logs string) error {
	return s.db.Model(&models.Job{}).Where("id = ?", jobID).Update("logs", logs).Error
}

// LoadProject loads a project by ID.
func (s *JobService) LoadProject(projectID uuid.UUID) (*models.Project, error) {
	var project models.Project
	if err := s.db.First(&project, projectID).Error; err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	return &project, nil
}

// LoadVersion loads a project version by ID.
func (s *JobService) LoadVersion(versionID uuid.UUID) (*models.ProjectVersion, error) {
	var version models.ProjectVersion
	if err := s.db.First(&version, versionID).Error; err != nil {
		return nil, fmt.Errorf("load version: %w", err)
	}
	return &version, nil
}
