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

func TestMemoryQueue_DefaultBufferSize(t *testing.T) {
	q := NewMemoryQueue(0) // should default to 100
	defer q.Close()

	// Should work — buffer size was corrected to 100
	job := newTestJob()
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue with default buffer: %v", err)
	}
}

func TestMemoryQueue_DequeueReleasesJobReferences(t *testing.T) {
	q := NewMemoryQueue(2)
	defer q.Close()
	first, second := newTestJob(), newTestJob()
	first.UserID = uuid.New()
	second.UserID = first.UserID
	for _, job := range []*models.Job{first, second} {
		if err := q.Enqueue(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	// Keep a view of the backing array: slicing past a processed job must
	// not retain its metadata while the next job waits for its turn.
	storage := q.pending[tenantKeyForJob(first)]
	for i, want := range []*models.Job{first, second} {
		got, err := q.Dequeue(context.Background())
		if err != nil || got != want {
			t.Fatalf("dequeue %d: got %v, err %v", i, got, err)
		}
		if storage[i] != nil {
			t.Fatalf("pending storage still retains dequeued job %d", i)
		}
	}
	if len(q.pending) != 0 || len(q.tenantOrder) != 0 || q.pendingSize != 0 {
		t.Fatal("drained queue still retains pending state")
	}
}
