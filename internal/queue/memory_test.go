package queue

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

func newTestJob() *models.Job {
	return &models.Job{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Type:        models.JobTypeCreate,
		Status:      models.JobStatusPending,
	}
}

func TestMemoryQueue_EnqueueDequeue(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := newTestJob()

	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if got.ID != job.ID {
		t.Errorf("expected job ID %s, got %s", job.ID, got.ID)
	}
}

func TestMemoryQueue_EnqueueRequiresID(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := &models.Job{} // no ID

	err := q.Enqueue(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for job without ID")
	}
}

func TestMemoryQueue_DequeueBlocksUntilJob(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Dequeue on empty queue should block until context expires
	_, err := q.Dequeue(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestMemoryQueue_DequeueAfterCloseDrainsPendingJob(t *testing.T) {
	q := NewMemoryQueue(10)

	job := newTestJob()
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue pending job after close: %v", err)
	}
	if got.ID != job.ID {
		t.Fatalf("expected job ID %s, got %s", job.ID, got.ID)
	}

	got, err = q.Dequeue(context.Background())
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled after draining closed queue, got job=%v err=%v", got, err)
	}
}

func TestMemoryQueue_FIFO(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job1 := newTestJob()
	job2 := newTestJob()
	job3 := newTestJob()

	q.Enqueue(context.Background(), job1)
	q.Enqueue(context.Background(), job2)
	q.Enqueue(context.Background(), job3)

	got1, _ := q.Dequeue(context.Background())
	got2, _ := q.Dequeue(context.Background())
	got3, _ := q.Dequeue(context.Background())

	if got1.ID != job1.ID || got2.ID != job2.ID || got3.ID != job3.ID {
		t.Errorf("expected FIFO order: %s,%s,%s got %s,%s,%s",
			job1.ID, job2.ID, job3.ID, got1.ID, got2.ID, got3.ID)
	}
}

func TestMemoryQueue_FairAcrossTenants(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	userA := uuid.New()
	userB := uuid.New()
	jobA1 := newTestJob()
	jobA1.UserID = userA
	jobA2 := newTestJob()
	jobA2.UserID = userA
	jobA3 := newTestJob()
	jobA3.UserID = userA
	jobB1 := newTestJob()
	jobB1.UserID = userB

	for _, job := range []*models.Job{jobA1, jobA2, jobA3, jobB1} {
		if err := q.Enqueue(context.Background(), job); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	got1, _ := q.Dequeue(context.Background())
	got2, _ := q.Dequeue(context.Background())
	got3, _ := q.Dequeue(context.Background())
	got4, _ := q.Dequeue(context.Background())

	if got1.ID != jobA1.ID || got2.ID != jobB1.ID || got3.ID != jobA2.ID || got4.ID != jobA3.ID {
		t.Fatalf("expected fair tenant order A1,B1,A2,A3 got %s,%s,%s,%s",
			got1.ID, got2.ID, got3.ID, got4.ID)
	}
}

func TestMemoryQueue_GetStatus(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := newTestJob()
	q.Enqueue(context.Background(), job)

	got, err := q.GetStatus(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if got.Status != models.JobStatusPending {
		t.Errorf("expected status pending, got %s", got.Status)
	}
}

func TestMemoryQueue_GetStatus_NotFound(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	_, err := q.GetStatus(context.Background(), uuid.New())
	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestMemoryQueue_UpdateStatus(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := newTestJob()
	q.Enqueue(context.Background(), job)

	err := q.UpdateStatus(context.Background(), job.ID, models.JobStatusRunning, "starting...")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := q.GetStatus(context.Background(), job.ID)
	if got.Status != models.JobStatusRunning {
		t.Errorf("expected running, got %s", got.Status)
	}
	if got.Logs != "starting..." {
		t.Errorf("expected logs 'starting...', got %q", got.Logs)
	}
}

func TestMemoryQueue_UpdateStatus_AppendLogs(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := newTestJob()
	q.Enqueue(context.Background(), job)

	q.UpdateStatus(context.Background(), job.ID, models.JobStatusRunning, "line1")
	q.UpdateStatus(context.Background(), job.ID, models.JobStatusRunning, "line2")

	got, _ := q.GetStatus(context.Background(), job.ID)
	if got.Logs != "line1\nline2" {
		t.Errorf("expected appended logs, got %q", got.Logs)
	}
}

func TestMemoryQueue_Complete(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := newTestJob()
	q.Enqueue(context.Background(), job)

	err := q.Complete(context.Background(), job.ID, "done")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	got, _ := q.GetStatus(context.Background(), job.ID)
	if got.Status != models.JobStatusCompleted {
		t.Errorf("expected completed, got %s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestMemoryQueue_Fail(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := newTestJob()
	q.Enqueue(context.Background(), job)

	err := q.Fail(context.Background(), job.ID, "something broke", "error logs")
	if err != nil {
		t.Fatalf("fail: %v", err)
	}

	got, _ := q.GetStatus(context.Background(), job.ID)
	if got.Status != models.JobStatusFailed {
		t.Errorf("expected failed, got %s", got.Status)
	}
	if got.Error != "something broke" {
		t.Errorf("expected error message, got %q", got.Error)
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestMemoryQueue_Complete_NotFound(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	err := q.Complete(context.Background(), uuid.New(), "")
	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestMemoryQueue_Fail_NotFound(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	err := q.Fail(context.Background(), uuid.New(), "err", "")
	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestMemoryQueue_DefaultBufferSize(t *testing.T) {
	q := NewMemoryQueue(0) // should default to 100
	defer q.Close()

	// Should work — buffer size was corrected to 100
	job := newTestJob()
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue with default buffer: %v", err)
	}
}

func TestMemoryQueue_EnqueueStoresCopy(t *testing.T) {
	q := NewMemoryQueue(10)
	defer q.Close()

	job := newTestJob()
	q.Enqueue(context.Background(), job)

	// Mutate the original
	job.Status = models.JobStatusRunning

	// The stored copy should still be pending
	got, _ := q.GetStatus(context.Background(), job.ID)
	if got.Status != models.JobStatusPending {
		t.Error("expected stored copy to be independent of original pointer")
	}
}
