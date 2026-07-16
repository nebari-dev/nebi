package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// JobService contains business logic for job operations.
type JobService struct {
	db      *gorm.DB
	isLocal bool
}

// NewJobService creates a new JobService. In local mode job visibility is
// not restricted by workspace ownership: the whole machine belongs to one
// person and every request runs as the synthetic local-user, so ownership
// filtering would hide jobs for workspaces created under a different mode.
func NewJobService(db *gorm.DB, isLocal bool) *JobService {
	return &JobService{db: db, isLocal: isLocal}
}

// ListJobs returns jobs for workspaces owned by the given user, or all
// jobs in local mode.
func (s *JobService) ListJobs(userID uuid.UUID) ([]models.Job, error) {
	var jobs []models.Job
	query := s.db.
		Select("jobs.*").
		Joins("JOIN workspaces ON workspaces.id = jobs.workspace_id")
	if !s.isLocal {
		query = query.Where("workspaces.owner_id = ?", userID)
	}
	err := query.Order("jobs.created_at DESC").Find(&jobs).Error

	if err != nil {
		return nil, fmt.Errorf("fetch jobs: %w", err)
	}
	return jobs, nil
}

// GetJob returns a single job by ID. Outside local mode it verifies the
// user owns the workspace.
func (s *JobService) GetJob(jobID string, userID uuid.UUID) (*models.Job, error) {
	var job models.Job
	query := s.db.
		Select("jobs.*").
		Joins("JOIN workspaces ON workspaces.id = jobs.workspace_id").
		Where("jobs.id = ?", jobID)
	if !s.isLocal {
		query = query.Where("workspaces.owner_id = ?", userID)
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
// mode it verifies the user owns the workspace. Returns the job regardless
// of status (caller decides what to do with completed jobs).
func (s *JobService) GetJobForStreaming(jobID uuid.UUID, userID uuid.UUID) (*models.Job, error) {
	var job models.Job
	query := s.db.
		Select("jobs.*").
		Joins("JOIN workspaces ON workspaces.id = jobs.workspace_id").
		Where("jobs.id = ?", jobID)
	if !s.isLocal {
		query = query.Where("workspaces.owner_id = ?", userID)
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

// MarkPanicked sets a job as failed after a panic recovery.
func (s *JobService) MarkPanicked(job *models.Job, panicMsg string) {
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Status = models.JobStatusFailed
	job.Error = panicMsg
	s.db.Save(job)
}

// FlushLogs persists the current log content for a job.
func (s *JobService) FlushLogs(jobID uuid.UUID, logs string) error {
	return s.db.Model(&models.Job{}).Where("id = ?", jobID).Update("logs", logs).Error
}

// LoadWorkspace loads a workspace by ID.
func (s *JobService) LoadWorkspace(workspaceID uuid.UUID) (*models.Workspace, error) {
	var ws models.Workspace
	if err := s.db.First(&ws, workspaceID).Error; err != nil {
		return nil, fmt.Errorf("load workspace: %w", err)
	}
	return &ws, nil
}

// LoadVersion loads a workspace version by ID.
func (s *JobService) LoadVersion(versionID uuid.UUID) (*models.WorkspaceVersion, error) {
	var version models.WorkspaceVersion
	if err := s.db.First(&version, versionID).Error; err != nil {
		return nil, fmt.Errorf("load version: %w", err)
	}
	return &version, nil
}
