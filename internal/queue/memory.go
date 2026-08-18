package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

// MemoryQueue implements an in-memory job queue
type MemoryQueue struct {
	jobs        map[uuid.UUID]*models.Job
	pending     map[string][]*models.Job
	tenantOrder []string
	pendingSize int
	bufferSize  int
	notify      chan struct{}
	closed      bool
	mu          sync.RWMutex
}

// NewMemoryQueue creates a new in-memory queue
func NewMemoryQueue(bufferSize int) *MemoryQueue {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	q := &MemoryQueue{
		jobs:       make(map[uuid.UUID]*models.Job),
		pending:    make(map[string][]*models.Job),
		bufferSize: bufferSize,
		notify:     make(chan struct{}, 1),
	}

	slog.Info("Initialized in-memory job queue", "buffer_size", bufferSize)
	return q
}

// Enqueue adds a job to the queue
func (q *MemoryQueue) Enqueue(ctx context.Context, job *models.Job) error {
	if job.ID == uuid.Nil {
		return fmt.Errorf("job must have an ID")
	}

	jobID := job.ID
	jobType := job.Type
	tenant := tenantKeyForJob(job)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return fmt.Errorf("queue is closed")
		}
		if q.pendingSize < q.bufferSize {
			// Store a copy in the jobs map (independent of the pointer sent to workers)
			jobCopy := *job
			q.jobs[job.ID] = &jobCopy
			if len(q.pending[tenant]) == 0 {
				q.tenantOrder = append(q.tenantOrder, tenant)
			}
			q.pending[tenant] = append(q.pending[tenant], job)
			q.pendingSize++
			q.signalLocked()
			q.mu.Unlock()
			slog.Debug("Job enqueued", "job_id", jobID, "type", jobType, "tenant", tenant)
			return nil
		}
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("queue is full, could not enqueue job %s", jobID)
		case <-ticker.C:
		}
	}
}

// Dequeue retrieves the next job from the queue
func (q *MemoryQueue) Dequeue(ctx context.Context) (*models.Job, error) {
	for {
		q.mu.Lock()
		job := q.nextJobLocked()
		closed := q.closed
		q.mu.Unlock()
		if job != nil {
			slog.Debug("Job dequeued", "job_id", job.ID, "type", job.Type, "tenant", tenantKeyForJob(job))
			return job, nil
		}
		if closed {
			return nil, context.Canceled
		}

		select {
		case <-q.notify:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (q *MemoryQueue) nextJobLocked() *models.Job {
	for len(q.tenantOrder) > 0 {
		tenant := q.tenantOrder[0]
		jobs := q.pending[tenant]
		if len(jobs) == 0 {
			delete(q.pending, tenant)
			q.tenantOrder = q.tenantOrder[1:]
			continue
		}

		job := jobs[0]
		jobs = jobs[1:]
		q.pendingSize--
		if len(jobs) == 0 {
			delete(q.pending, tenant)
			q.tenantOrder = q.tenantOrder[1:]
		} else {
			q.pending[tenant] = jobs
			copy(q.tenantOrder, q.tenantOrder[1:])
			q.tenantOrder[len(q.tenantOrder)-1] = tenant
		}
		q.signalLocked()
		return job
	}
	return nil
}

func (q *MemoryQueue) signalLocked() {
	if q.closed {
		return
	}
	select {
	case q.notify <- struct{}{}:
	default:
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
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.notify)
	}
	slog.Info("Memory queue closed")
	return nil
}
