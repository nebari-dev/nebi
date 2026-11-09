package worker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
)

// blockingDispatcher claims create_version jobs and blocks until cancelled.
type blockingDispatcher struct {
	started chan struct{}
}

func (d *blockingDispatcher) Handles(job *models.Job) bool {
	return job.Type == models.JobTypeCreateVersion
}

func (d *blockingDispatcher) Run(ctx context.Context, job *models.Job, w io.Writer) error {
	n := 9
	job.VersionNumber = &n
	close(d.started)
	<-ctx.Done()
	return ctx.Err()
}

// TestCancelJobMarksCancelled covers issue #649: a running job dispatched
// to the client stack is cancelled through its context and stored as
// "cancelled", and the worker does not run legacy cleanup for it.
func TestCancelJobMarksCancelled(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	_, job := newTestProject(t, db, exec, "cancel-me", models.JobTypeCreateVersion, nil)

	d := &blockingDispatcher{started: make(chan struct{})}
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1, WithDispatcher(d))

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.processJob(context.Background(), job)
	}()
	select {
	case <-d.started:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never ran")
	}
	if !w.CancelJob(job.ID) {
		t.Fatal("CancelJob returned false for a running job")
	}
	<-done

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.JobStatusCancelled {
		t.Fatalf("status = %q, want cancelled", stored.Status)
	}
	if stored.VersionNumber == nil || *stored.VersionNumber != 9 {
		t.Fatalf("version link not persisted: %v", stored.VersionNumber)
	}
	if exec.cleanupCalls != 0 {
		t.Fatalf("legacy cleanup ran %d times for a dispatched job", exec.cleanupCalls)
	}
	if w.CancelJob(job.ID) {
		t.Fatal("CancelJob must return false once the job finished")
	}
}

// TestPendingCancelledJobIsSkipped: a job cancelled while queued never runs.
func TestPendingCancelledJobIsSkipped(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	_, job := newTestProject(t, db, exec, "skip-me", models.JobTypeCreateVersion, nil)
	db.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", models.JobStatusCancelled)

	d := &blockingDispatcher{started: make(chan struct{})}
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1, WithDispatcher(d))
	w.processJob(context.Background(), job)

	select {
	case <-d.started:
		t.Fatal("cancelled pending job was run")
	default:
	}
}
