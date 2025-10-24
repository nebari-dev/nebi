package queue

import (
	"context"
	"errors"

	"github.com/aktech/darb/internal/models"
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
	GetStatus(ctx context.Context, jobID uint) (*models.Job, error)

	// UpdateStatus updates the status of a job
	UpdateStatus(ctx context.Context, jobID uint, status models.JobStatus, logs string) error

	// Complete marks a job as completed
	Complete(ctx context.Context, jobID uint, logs string) error

	// Fail marks a job as failed
	Fail(ctx context.Context, jobID uint, errorMsg string, logs string) error

	// Close closes the queue and releases resources
	Close() error
}
