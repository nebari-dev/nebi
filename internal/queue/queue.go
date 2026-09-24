package queue

import (
	"context"

	"github.com/nebari-dev/nebi/internal/models"
)

// Queue represents a job queue interface
type Queue interface {
	// Enqueue adds a job to the queue
	Enqueue(ctx context.Context, job *models.Job) error

	// Dequeue retrieves the next job from the queue
	Dequeue(ctx context.Context) (*models.Job, error)

	// Close closes the queue and releases resources
	Close() error
}
