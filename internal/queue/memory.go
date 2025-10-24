package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aktech/darb/internal/models"
	"github.com/google/uuid"
)

// MemoryQueue implements an in-memory job queue
type MemoryQueue struct {
	jobs    map[uuid.UUID]*models.Job
	jobChan chan *models.Job
	mu      sync.RWMutex
}

// NewMemoryQueue creates a new in-memory queue
func NewMemoryQueue(bufferSize int) *MemoryQueue {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	q := &MemoryQueue{
		jobs:    make(map[uuid.UUID]*models.Job),
		jobChan: make(chan *models.Job, bufferSize),
	}

	slog.Info("Initialized in-memory job queue", "buffer_size", bufferSize)
	return q
}

// Enqueue adds a job to the queue
func (q *MemoryQueue) Enqueue(ctx context.Context, job *models.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if job.ID == uuid.Nil {
		return fmt.Errorf("job must have an ID")
	}

	// Store job in memory
	q.jobs[job.ID] = job

	// Send to channel (non-blocking with timeout)
	select {
	case q.jobChan <- job:
		slog.Debug("Job enqueued", "job_id", job.ID, "type", job.Type)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("queue is full, could not enqueue job %d", job.ID)
	}
}

// Dequeue retrieves the next job from the queue
func (q *MemoryQueue) Dequeue(ctx context.Context) (*models.Job, error) {
	select {
	case job := <-q.jobChan:
		slog.Debug("Job dequeued", "job_id", job.ID, "type", job.Type)
		return job, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetStatus retrieves the current status of a job
func (q *MemoryQueue) GetStatus(ctx context.Context, jobID uuid.UUID) (*models.Job, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	job, exists := q.jobs[jobID]
	if !exists {
		return nil, ErrJobNotFound
	}

	return job, nil
}

// UpdateStatus updates the status of a job
func (q *MemoryQueue) UpdateStatus(ctx context.Context, jobID uuid.UUID, status models.JobStatus, logs string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, exists := q.jobs[jobID]
	if !exists {
		return ErrJobNotFound
	}

	job.Status = status
	if logs != "" {
		if job.Logs != "" {
			job.Logs += "\n" + logs
		} else {
			job.Logs = logs
		}
	}

	slog.Debug("Job status updated", "job_id", jobID, "status", status)
	return nil
}

// Complete marks a job as completed
func (q *MemoryQueue) Complete(ctx context.Context, jobID uuid.UUID, logs string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, exists := q.jobs[jobID]
	if !exists {
		return ErrJobNotFound
	}

	job.Status = models.JobStatusCompleted
	now := time.Now()
	job.CompletedAt = &now
	if logs != "" {
		if job.Logs != "" {
			job.Logs += "\n" + logs
		} else {
			job.Logs = logs
		}
	}

	slog.Info("Job completed", "job_id", jobID, "type", job.Type)
	return nil
}

// Fail marks a job as failed
func (q *MemoryQueue) Fail(ctx context.Context, jobID uuid.UUID, errorMsg string, logs string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, exists := q.jobs[jobID]
	if !exists {
		return ErrJobNotFound
	}

	job.Status = models.JobStatusFailed
	job.Error = errorMsg
	now := time.Now()
	job.CompletedAt = &now
	if logs != "" {
		if job.Logs != "" {
			job.Logs += "\n" + logs
		} else {
			job.Logs = logs
		}
	}

	slog.Error("Job failed", "job_id", jobID, "type", job.Type, "error", errorMsg)
	return nil
}

// Close closes the queue and releases resources
func (q *MemoryQueue) Close() error {
	close(q.jobChan)
	slog.Info("Memory queue closed")
	return nil
}
