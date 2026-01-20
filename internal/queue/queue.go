package queue

import (
	"context"
	"errors"

	"github.com/openteams-ai/darb/internal/models"
	"github.com/google/uuid"
)

// ErrJobNotFound is returned when a job is not found
var ErrJobNotFound = errors.New("job not found")

// Queue represents a job queue interface
type Queue interface {
	// Enqueue adds a job to the queue
	Enqueue(ctx context.Context, job *models.Job) error

	// Dequeue retrieves the next job from the queue
	Dequeue(ctx context.Context) (*models.Job, error)

	// GetStatus retrieves the current status of a job
	GetStatus(ctx context.Context, jobID uuid.UUID) (*models.Job, error)

	// UpdateStatus updates the status of a job
	UpdateStatus(ctx context.Context, jobID uuid.UUID, status models.JobStatus, logs string) error

	// Complete marks a job as completed
	Complete(ctx context.Context, jobID uuid.UUID, logs string) error

	// Fail marks a job as failed
	Fail(ctx context.Context, jobID uuid.UUID, errorMsg string, logs string) error

	// Close closes the queue and releases resources
	Close() error
}
