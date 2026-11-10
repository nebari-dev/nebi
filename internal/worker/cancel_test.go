package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
)

// loggingDispatcher writes a line, then blocks until cancelled.
type loggingDispatcher struct {
	started chan struct{}
	release chan struct{}
}

func (d *loggingDispatcher) Handles(job *models.Job) bool {
	return job.Type == models.JobTypeCreateVersion
}

func (d *loggingDispatcher) Run(ctx context.Context, job *models.Job, w io.Writer) error {
	_, _ = io.WriteString(w, "hello from the job\n")
	close(d.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.release:
		_, _ = io.WriteString(w, "bye\n")
		return nil
	}
}

// TestExternalCancelStopsRunningJob: a cancel recorded in the database by
// another process (the CLI) stops a job running in this worker.
func TestExternalCancelStopsRunningJob(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	_, job := newTestProject(t, db, exec, "external-cancel", models.JobTypeCreateVersion, nil)

	d := &loggingDispatcher{started: make(chan struct{}), release: make(chan struct{})}
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1,
		WithDispatcher(d), WithCancelPollInterval(10*time.Millisecond))

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.processJob(context.Background(), job)
	}()
	<-d.started
	changed, err := jobSvc.CancelActive(job.ID)
	if err != nil || !changed {
		t.Fatalf("CancelActive = %v, %v", changed, err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("job not stopped by the external cancel")
	}
	var stored models.Job
	db.First(&stored, "id = ?", job.ID)
	if stored.Status != models.JobStatusCancelled || !strings.Contains(stored.Logs, "hello from the job") {
		t.Fatalf("stored = %s %q", stored.Status, stored.Logs)
	}
}

// TestTeamModeDoesNotWatchRows: the team server never polls job rows for
// external cancels.
func TestTeamModeDoesNotWatchRows(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTestMode(t, false)
	_, job := newTestProject(t, db, exec, "team-no-watch", models.JobTypeCreateVersion, nil)
	d := &loggingDispatcher{started: make(chan struct{}), release: make(chan struct{})}
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1,
		WithDispatcher(d), WithCancelPollInterval(5*time.Millisecond))
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.processJob(context.Background(), job)
	}()
	<-d.started
	if _, err := jobSvc.CancelActive(job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("team-mode job was stopped by a row change")
	case <-time.After(100 * time.Millisecond):
	}
	close(d.release)
	<-done
}

// TestCancelBeforeStartWins: StartJob never overwrites a cancel that
// landed while the job was queued.
func TestCancelBeforeStartWins(t *testing.T) {
	db, _, jobSvc, exec := setupWorkerTest(t)
	_, job := newTestProject(t, db, exec, "cancel-before-start", models.JobTypeCreateVersion, nil)
	if changed, err := jobSvc.CancelActive(job.ID); err != nil || !changed {
		t.Fatalf("CancelActive = %v, %v", changed, err)
	}
	if jobSvc.StartJob(job) {
		t.Fatal("StartJob started a cancelled job")
	}
	var stored models.Job
	db.First(&stored, "id = ?", job.ID)
	if stored.Status != models.JobStatusCancelled {
		t.Fatalf("status = %s", stored.Status)
	}
	// Finished jobs are not cancelled.
	db.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", models.JobStatusCompleted)
	if changed, _ := jobSvc.CancelActive(job.ID); changed {
		t.Fatal("CancelActive changed a finished job")
	}
}

// TestBrokerHistoryForLateSubscriber: a subscriber joining a running job
// gets the history and then live chunks.
func TestBrokerHistoryForLateSubscriber(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	_, job := newTestProject(t, db, exec, "late-subscriber", models.JobTypeCreateVersion, nil)
	d := &loggingDispatcher{started: make(chan struct{}), release: make(chan struct{})}
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1, WithDispatcher(d))

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.processJob(context.Background(), job)
	}()
	<-d.started
	history, ch, ok := w.GetBroker().SubscribeWithHistory(job.ID)
	if !ok || !strings.Contains(history, "hello from the job") {
		t.Fatalf("history = %q, ok = %v", history, ok)
	}
	close(d.release)
	var live strings.Builder
	for chunk := range ch {
		live.WriteString(chunk)
	}
	<-done
	if !strings.Contains(live.String(), "bye") || !strings.Contains(live.String(), "[COMPLETED]") || strings.Contains(live.String(), "hello") {
		t.Fatalf("live = %q", live.String())
	}
	if _, _, ok := w.GetBroker().SubscribeWithHistory(job.ID); ok {
		t.Fatal("finished job still tracked by the broker")
	}
}

// failingDispatcher writes a line and fails.
type failingDispatcher struct{}

func (failingDispatcher) Handles(job *models.Job) bool {
	return job.Type == models.JobTypeCreateVersion
}

func (failingDispatcher) Run(_ context.Context, _ *models.Job, w io.Writer) error {
	_, _ = io.WriteString(w, "solving\n")
	return errors.New("writing pixi.toml: permission denied")
}

// TestStoredLogEndsWithFailureReason: the stored log of a failed job ends
// with the error, so the job page ("Please see logs for details") shows
// why it failed after the live stream is gone.
func TestStoredLogEndsWithFailureReason(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	_, job := newTestProject(t, db, exec, "failure-reason", models.JobTypeCreateVersion, nil)
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1,
		WithDispatcher(failingDispatcher{}))
	w.processJob(context.Background(), job)

	var stored models.Job
	db.First(&stored, "id = ?", job.ID)
	if stored.Status != models.JobStatusFailed {
		t.Fatalf("status = %s", stored.Status)
	}
	if !strings.HasSuffix(stored.Logs, "\n[ERROR] Job failed: writing pixi.toml: permission denied\n") {
		t.Fatalf("stored log = %q", stored.Logs)
	}
}

// TestStoredLogEndsWithCancelMarker: a job cancelled through the worker
// keeps the "[CANCELLED]" line in its stored log.
func TestStoredLogEndsWithCancelMarker(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	_, job := newTestProject(t, db, exec, "cancel-marker", models.JobTypeCreateVersion, nil)
	d := &loggingDispatcher{started: make(chan struct{}), release: make(chan struct{})}
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1, WithDispatcher(d))
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.processJob(context.Background(), job)
	}()
	<-d.started
	if !w.CancelJob(job.ID) {
		t.Fatal("CancelJob = false for a running job")
	}
	<-done
	var stored models.Job
	db.First(&stored, "id = ?", job.ID)
	if stored.Status != models.JobStatusCancelled || !strings.HasSuffix(stored.Logs, "[CANCELLED] Job cancelled\n") {
		t.Fatalf("stored = %s %q", stored.Status, stored.Logs)
	}
}
