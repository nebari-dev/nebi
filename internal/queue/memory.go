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

// MemoryQueue schedules pending jobs in memory. JobService persists job status
// and logs in the database; the queue does not keep a second copy of that state.
type MemoryQueue struct {
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
		jobs[0] = nil // Release the reference held by the pending slice backing array.
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
