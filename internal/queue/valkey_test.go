package queue

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestValkeyQueue(t *testing.T) (*ValkeyQueue, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	db := newValkeyTestDB(t)

	q, err := NewValkeyQueue(server.Addr(), db)
	if err != nil {
		t.Fatalf("NewValkeyQueue: %v", err)
	}
	t.Cleanup(func() {
		_ = q.Close()
	})
	return q, server
}

func newValkeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "queue.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Workspace{}, &models.Job{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newValkeyTestJob(userID uuid.UUID) *models.Job {
	return &models.Job{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		UserID:      userID,
		Type:        models.JobTypeCreate,
		Status:      models.JobStatusPending,
	}
}

func dequeueValkeyTestJob(t *testing.T, q *ValkeyQueue) *models.Job {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	job, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	return job
}

func TestValkeyQueue_FairAcrossTenants(t *testing.T) {
	q, _ := newTestValkeyQueue(t)
	userA := uuid.New()
	userB := uuid.New()

	jobA1 := newValkeyTestJob(userA)
	jobA2 := newValkeyTestJob(userA)
	jobA3 := newValkeyTestJob(userA)
	jobB1 := newValkeyTestJob(userB)
	for _, job := range []*models.Job{jobA1, jobA2, jobA3, jobB1} {
		if err := q.Enqueue(context.Background(), job); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	got1 := dequeueValkeyTestJob(t, q)
	got2 := dequeueValkeyTestJob(t, q)
	got3 := dequeueValkeyTestJob(t, q)
	got4 := dequeueValkeyTestJob(t, q)

	if got1.ID != jobA1.ID || got2.ID != jobB1.ID || got3.ID != jobA2.ID || got4.ID != jobA3.ID {
		t.Fatalf("expected fair tenant order A1,B1,A2,A3 got %s,%s,%s,%s",
			got1.ID, got2.ID, got3.ID, got4.ID)
	}
}

func TestValkeyQueue_StaleTenantEntryDoesNotStrandFutureJobs(t *testing.T) {
	q, server := newTestValkeyQueue(t)
	userID := uuid.New()
	tenant := "user:" + userID.String()
	if _, err := server.ZAdd(q.tenantsKey, 1, tenant); err != nil {
		t.Fatalf("seed stale tenant entry: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	_, err := q.Dequeue(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected empty queue deadline after stale tenant entry, got %v", err)
	}

	job := newValkeyTestJob(userID)
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue after stale entry: %v", err)
	}
	got := dequeueValkeyTestJob(t, q)
	if got.ID != job.ID {
		t.Fatalf("expected job %s after stale entry, got %s", job.ID, got.ID)
	}
}

func TestValkeyQueue_ReconcilesLegacyQueueOnStartup(t *testing.T) {
	server := miniredis.RunT(t)
	db := newValkeyTestDB(t)
	job := newValkeyTestJob(uuid.New())
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := server.RPush(legacyQueueKey, job.ID.String()); err != nil {
		t.Fatalf("seed legacy queue: %v", err)
	}

	q, err := NewValkeyQueue(server.Addr(), db)
	if err != nil {
		t.Fatalf("NewValkeyQueue: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	got := dequeueValkeyTestJob(t, q)
	if got.ID != job.ID {
		t.Fatalf("expected migrated legacy job %s, got %s", job.ID, got.ID)
	}
	if !server.Exists(legacyQueueKey) {
		return
	}
	if values, err := server.List(legacyQueueKey); err != nil {
		t.Fatalf("legacy list: %v", err)
	} else if len(values) != 0 {
		t.Fatalf("expected legacy list drained, got %v", values)
	}
}

func TestValkeyQueue_ReconcilesStalePendingDBJobsOnStartup(t *testing.T) {
	server := miniredis.RunT(t)
	db := newValkeyTestDB(t)
	job := newValkeyTestJob(uuid.New())
	job.CreatedAt = time.Now().Add(-pendingReconcileAge - time.Second)
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	q, err := NewValkeyQueue(server.Addr(), db)
	if err != nil {
		t.Fatalf("NewValkeyQueue: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	got := dequeueValkeyTestJob(t, q)
	if got.ID != job.ID {
		t.Fatalf("expected reconciled pending job %s, got %s", job.ID, got.ID)
	}
}

func TestValkeyQueue_SkipsNonPendingTransportEntries(t *testing.T) {
	q, _ := newTestValkeyQueue(t)
	job := newValkeyTestJob(uuid.New())
	job.Status = models.JobStatusCompleted
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	_, err := q.Dequeue(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected stale completed job to be skipped, got %v", err)
	}
}
