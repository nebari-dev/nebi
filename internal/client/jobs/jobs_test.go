package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newRunner(t *testing.T) *Memory {
	t.Helper()
	m := NewMemory()
	t.Cleanup(m.Close)
	return m
}

func waitCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSubmitSuccessAndFailure(t *testing.T) {
	m := newRunner(t)
	project := uuid.New()

	ok, err := m.Submit(context.Background(), project, TypeCreate, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if ok.Status != StatusQueued || ok.ProjectID != project || ok.Type != TypeCreate {
		t.Fatalf("unexpected queued job: %+v", ok)
	}
	bad, err := m.Submit(context.Background(), project, TypeInstall, func(context.Context) error { return errors.New("boom") })
	if err != nil {
		t.Fatal(err)
	}

	got, err := m.Wait(waitCtx(t), ok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusSucceeded || got.Error != "" || got.StartedAt == nil || got.FinishedAt == nil || got.Active() {
		t.Fatalf("want succeeded job, got %+v", got)
	}
	got, err = m.Wait(waitCtx(t), bad.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.Error != "boom" {
		t.Fatalf("want failed job with error, got %+v", got)
	}
}

func TestPanicBecomesFailure(t *testing.T) {
	m := newRunner(t)
	j, _ := m.Submit(context.Background(), uuid.New(), TypeCreate, func(context.Context) error { panic("oops") })
	got, err := m.Wait(waitCtx(t), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.Error != "job panicked: oops" {
		t.Fatalf("got %+v", got)
	}
}

func TestActiveWhileRunningAndProjectJobsOrder(t *testing.T) {
	m := newRunner(t)
	project, other := uuid.New(), uuid.New()
	release := make(chan struct{})
	started := make(chan struct{})

	first, _ := m.Submit(context.Background(), project, TypeCreate, func(context.Context) error {
		close(started)
		<-release
		return nil
	})
	m.Submit(context.Background(), other, TypeCreate, func(context.Context) error { return nil })
	second, _ := m.Submit(context.Background(), project, TypeDeleteFiles, func(context.Context) error { return nil })

	<-started
	if got, _ := m.Get(first.ID); !got.Active() || got.Status != StatusRunning {
		t.Fatalf("first job should be running, got %+v", got)
	}

	list, err := m.ProjectJobs(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("want [first, second] for the project in submission order, got %+v", list)
	}
	close(release)
	if got, _ := m.Wait(waitCtx(t), first.ID); got.Status != StatusSucceeded {
		t.Fatalf("got %+v", got)
	}
}

func TestContextCarriesJobIDAndStage(t *testing.T) {
	m := newRunner(t)
	var seen uuid.UUID
	version := uuid.New()
	j, _ := m.Submit(context.Background(), uuid.New(), TypeCreate, func(ctx context.Context) error {
		id, ok := IDFromContext(ctx)
		if !ok {
			return errors.New("no job id in context")
		}
		seen = id
		ReportStage(ctx, "lock")
		ReportStage(ctx, "install")
		ReportVersion(ctx, version)
		return nil
	})
	got, _ := m.Wait(waitCtx(t), j.ID)
	if got.Status != StatusSucceeded {
		t.Fatalf("got %+v", got)
	}
	if seen != j.ID {
		t.Fatalf("IDFromContext = %s, want %s", seen, j.ID)
	}
	if got.Stage != "install" {
		t.Fatalf("Stage = %q, want last reported stage %q", got.Stage, "install")
	}
	if got.VersionID == nil || *got.VersionID != version {
		t.Fatalf("VersionID = %v, want %s", got.VersionID, version)
	}
	if _, ok := IDFromContext(context.Background()); ok {
		t.Fatal("IDFromContext outside a job should report false")
	}
	ReportStage(context.Background(), "ignored") // must not panic
	ReportVersion(context.Background(), version)
}

func TestCloseCancelsAndRejects(t *testing.T) {
	m := NewMemory()
	started := make(chan struct{})
	j, _ := m.Submit(context.Background(), uuid.New(), TypeCreate, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started
	m.Close()

	got, err := m.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed {
		t.Fatalf("cancelled job should have failed, got %+v", got)
	}
	if _, err := m.Submit(context.Background(), uuid.New(), TypeCreate, func(context.Context) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("Submit after Close: want ErrClosed, got %v", err)
	}
}

func TestUnknownJobAndNilBody(t *testing.T) {
	m := newRunner(t)
	if _, err := m.Get(uuid.New()); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Get: want ErrUnknownJob, got %v", err)
	}
	if _, err := m.Wait(waitCtx(t), uuid.New()); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Wait: want ErrUnknownJob, got %v", err)
	}
	if _, err := m.Submit(context.Background(), uuid.New(), TypeCreate, nil); err == nil {
		t.Fatal("Submit with nil body should fail")
	}
}
