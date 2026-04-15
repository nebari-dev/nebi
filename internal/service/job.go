package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// JobService contains business logic for job operations.
type JobService struct {
	db *gorm.DB
}

// NewJobService creates a new JobService.
func NewJobService(db *gorm.DB) *JobService {
	return &JobService{db: db}
}

// ListJobs returns all jobs for workspaces owned by the given user.
func (s *JobService) ListJobs(userID uuid.UUID) ([]models.Job, error) {
	var jobs []models.Job
	err := s.db.
		Select("jobs.*").
		Joins("JOIN workspaces ON workspaces.id = jobs.workspace_id").
		Where("workspaces.owner_id = ?", userID).
		Order("jobs.created_at DESC").
		Find(&jobs).Error

	if err != nil {
		return nil, fmt.Errorf("fetch jobs: %w", err)
	}
	return jobs, nil
}

// GetJob returns a single job by ID, verifying the user owns the workspace.
func (s *JobService) GetJob(jobID string, userID uuid.UUID) (*models.Job, error) {
	var job models.Job
	err := s.db.
		Select("jobs.*").
		Joins("JOIN workspaces ON workspaces.id = jobs.workspace_id").
		Where("jobs.id = ? AND workspaces.owner_id = ?", jobID, userID).
		First(&job).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch job: %w", err)
	}
	return &job, nil
}

// GetJobForStreaming returns a job by ID with ownership check, for SSE streaming.
// Returns the job regardless of status (caller decides what to do with completed jobs).
func (s *JobService) GetJobForStreaming(jobID uuid.UUID, userID uuid.UUID) (*models.Job, error) {
	var job models.Job
	err := s.db.
		Select("jobs.*").
		Joins("JOIN workspaces ON workspaces.id = jobs.workspace_id").
		Where("jobs.id = ? AND workspaces.owner_id = ?", jobID, userID).
		First(&job).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch job: %w", err)
	}
	return &job, nil
}
