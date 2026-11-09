package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/nebari-dev/nebi/internal/worker"
	"gorm.io/gorm"
)

// Test-only job types handled through the client dispatcher.
const (
	testLinesJob models.JobType = "test_lines" // writes numbered lines with pauses
	testBlockJob models.JobType = "test_block" // blocks until ctx ends
)

func init() {
	register(testLinesJob, func(ctx context.Context, rc RunContext) error {
		for i := 1; i <= 20; i++ {
			fmt.Fprintf(rc.Log, "line %d\n", i)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Millisecond):
			}
		}
		return nil
	})
	register(testBlockJob, func(ctx context.Context, rc RunContext) error {
		fmt.Fprintln(rc.Log, "waiting")
		<-ctx.Done()
		return ctx.Err()
	})
}

type harness struct {
	db  *gorm.DB
	sdk *sdk.Client
	svc *Service
	q   *queue.MemoryQueue
}

// newHarness builds a client stack on a temp database. With withQueue the
// service enqueues jobs (nebi-web/desktop); otherwise jobs run inline (CLI).
func newHarness(t *testing.T, withQueue bool) *harness {
	t.Helper()
	dir := t.TempDir()
	gdb, err := local.OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Storage.ProjectsDir = t.TempDir()
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c, err := sdk.New(shared.Deps{DB: gdb, Executor: exec, DataDir: dir, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{db: gdb, sdk: c}
	opts := Options{Logger: logger}
	if withQueue {
		h.q = queue.NewMemoryQueue(16)
		opts.Queue = h.q
	}
	h.svc = New(c, opts)
	h.svc.pollInterval = 20 * time.Millisecond
	return h
}

// startWorker runs the shared worker with this service's dispatcher.
func (h *harness) startWorker(t *testing.T) *worker.Worker {
	t.Helper()
	exec := h.sdk.Deps().Executor
	projSvc := service.New(h.db, h.q, exec, true, nil, nil, limits.Defaults())
	w := worker.New(h.q, exec, projSvc, service.NewJobService(h.db, true), slog.New(slog.NewTextHandler(io.Discard, nil)),
		limits.Defaults(), 2, worker.WithDispatcher(h.svc.Dispatcher()), worker.WithCancelPollInterval(20*time.Millisecond))
	h.svc.BindWorker(w)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = w.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		h.q.Close()
		<-done
	})
	return w
}

func (h *harness) project(t *testing.T, name string) *models.Project {
	t.Helper()
	p := &models.Project{Name: name, OwnerID: h.sdk.Deps().LocalUserID, Status: models.ProjectStatusReady}
	if err := h.db.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	return p
}

// job inserts a job row directly (status and timing under test control).
func (h *harness) job(t *testing.T, p *models.Project, typ models.JobType, st models.JobStatus, started time.Time, meta map[string]any) *models.Job {
	t.Helper()
	j := &models.Job{ProjectID: p.ID, Type: typ, Status: st, Metadata: meta, CreatedAt: started.Add(-time.Second)}
	if st != models.JobStatusPending {
		s := started
		j.StartedAt = &s
	}
	if st == models.JobStatusCompleted || st == models.JobStatusFailed || st == models.JobStatusCancelled {
		e := started.Add(30 * time.Second)
		j.CompletedAt = &e
	}
	if err := h.db.Create(j).Error; err != nil {
		t.Fatal(err)
	}
	return j
}

func (h *harness) waitStatus(t *testing.T, id uuid.UUID, want models.JobStatus) *shared.JobDetail {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		d, err := h.svc.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if d.Status == want {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s status %q, want %q (error %q)", id, d.Status, want, d.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func ids(list []shared.JobSummary) []uuid.UUID {
	out := make([]uuid.UUID, len(list))
	for i, j := range list {
		out[i] = j.ID
	}
	return out
}

func TestListFiltersSortAndNames(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()
	a := h.project(t, "alpha")
	b := h.project(t, "beta")
	base := time.Now().Add(-time.Hour)
	j1 := h.job(t, a, models.JobTypeCreate, models.JobStatusCompleted, base, nil)
	j2 := h.job(t, a, models.JobTypeInstallVersion, models.JobStatusFailed, base.Add(10*time.Minute), nil)
	j3 := h.job(t, b, models.JobTypeCreateVersion, models.JobStatusRunning, base.Add(20*time.Minute), nil)
	j4 := h.job(t, b, models.JobTypeUninstall, models.JobStatusPending, base.Add(30*time.Minute), nil)

	all, err := h.svc.List(ctx, shared.JobListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(all), []uuid.UUID{j4.ID, j3.ID, j2.ID, j1.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("default order = %v, want %v (newest start first, pending by creation)", got, want)
	}
	if all[0].ProjectName != "beta" || all[3].ProjectName != "alpha" {
		t.Fatalf("project names: %+v", all)
	}
	if all[2].TypeLabel != "Install version" || all[2].DurationMs == nil || *all[2].DurationMs != 30000 {
		t.Fatalf("summary fields: %+v", all[2])
	}

	asc, err := h.svc.List(ctx, shared.JobListFilter{Sort: "started_at", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(asc), []uuid.UUID{j1.ID, j2.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ascending+limit = %v, want %v", got, want)
	}

	byStatus, _ := h.svc.List(ctx, shared.JobListFilter{Statuses: []models.JobStatus{models.JobStatusRunning, models.JobStatusPending}})
	if got, want := ids(byStatus), []uuid.UUID{j4.ID, j3.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("status filter = %v, want %v", got, want)
	}
	byType, _ := h.svc.List(ctx, shared.JobListFilter{Types: []models.JobType{models.JobTypeCreate}})
	if len(byType) != 1 || byType[0].ID != j1.ID {
		t.Fatalf("type filter = %v", ids(byType))
	}
	byProject, _ := h.svc.List(ctx, shared.JobListFilter{ProjectID: &a.ID})
	if got, want := ids(byProject), []uuid.UUID{j2.ID, j1.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("project filter = %v, want %v", got, want)
	}

	// A deleted project keeps its name on its jobs.
	if err := h.db.Delete(&models.Project{}, "id = ?", a.ID).Error; err != nil {
		t.Fatal(err)
	}
	d, err := h.svc.Get(ctx, j1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.ProjectName != "alpha" {
		t.Fatalf("deleted project name = %q", d.ProjectName)
	}

	for _, bad := range []shared.JobListFilter{
		{Sort: "name"},
		{Statuses: []models.JobStatus{"done"}},
		{Types: []models.JobType{"nope"}},
		{Limit: -1},
	} {
		var verr *shared.ValidationError
		if _, err := h.svc.List(ctx, bad); !errors.As(err, &verr) {
			t.Fatalf("List(%+v) error = %v, want ValidationError", bad, err)
		}
	}
	if _, err := h.svc.Get(ctx, uuid.New()); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
}

func TestResolveJobID(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()
	p := h.project(t, "p")
	j := h.job(t, p, models.JobTypeCreate, models.JobStatusCompleted, time.Now(), nil)

	for _, ref := range []string{j.ID.String(), j.ID.String()[:8], strings.ToUpper(j.ID.String()[:6])} {
		got, err := h.svc.ResolveJobID(ctx, ref)
		if err != nil || got != j.ID {
			t.Fatalf("ResolveJobID(%q) = %v, %v", ref, got, err)
		}
	}
	if _, err := h.svc.ResolveJobID(ctx, "ab"); err == nil {
		t.Fatal("short prefix must be rejected")
	}
	if _, err := h.svc.ResolveJobID(ctx, "ffffffff"); !errors.Is(err, shared.ErrNotFound) && j.ID.String()[:8] != "ffffffff" {
		t.Fatalf("unknown prefix = %v", err)
	}
}

func TestCancelPendingAndFinished(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()
	p := h.project(t, "p")
	pending := h.job(t, p, models.JobTypeInstallVersion, models.JobStatusPending, time.Now(), nil)

	sum, err := h.svc.Cancel(ctx, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Status != models.JobStatusCancelled || sum.CompletedAt == nil {
		t.Fatalf("cancel pending = %+v", sum)
	}
	// Idempotent.
	if again, err := h.svc.Cancel(ctx, pending.ID); err != nil || again.Status != models.JobStatusCancelled {
		t.Fatalf("second cancel = %+v, %v", again, err)
	}
	done := h.job(t, p, models.JobTypeInstallVersion, models.JobStatusCompleted, time.Now(), nil)
	var cerr *shared.ConflictError
	if _, err := h.svc.Cancel(ctx, done.ID); !errors.As(err, &cerr) || cerr.Code != "job_finished" {
		t.Fatalf("cancel finished = %v, want job_finished conflict", err)
	}
	if _, err := h.svc.Cancel(ctx, uuid.New()); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("cancel missing = %v", err)
	}
}

// TestFollowLogsFromWorker: the live stream of a job run by the worker
// delivers every line exactly once, in order, and ends with the status.
func TestFollowLogsFromWorker(t *testing.T) {
	h := newHarness(t, true)
	h.startWorker(t)
	ctx := context.Background()
	p := h.project(t, "p")

	job, err := h.svc.Submit(ctx, shared.JobRequest{ProjectID: p.ID, Type: testLinesJob})
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	final, err := h.svc.FollowLogs(ctx, job.ID, func(chunk string) error {
		got.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != models.JobStatusCompleted {
		t.Fatalf("final status %q", final.Status)
	}
	stored := h.waitStatus(t, job.ID, models.JobStatusCompleted).Logs
	if !strings.HasPrefix(got.String(), stored) {
		t.Fatalf("stream %q does not start with stored log %q", got.String(), stored)
	}
	for i := 1; i <= 20; i++ {
		if n := strings.Count(got.String(), fmt.Sprintf("line %d\n", i)); n != 1 {
			t.Fatalf("line %d seen %d times in %q", i, n, got.String())
		}
	}
}

// TestFollowLogsPollsOtherProcess: a job run elsewhere (the CLI) is
// followed through its stored log.
func TestFollowLogsPollsOtherProcess(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()
	p := h.project(t, "p")
	j := h.job(t, p, models.JobTypeInstallVersion, models.JobStatusRunning, time.Now(), nil)

	go func() {
		logs := ""
		for i := 1; i <= 5; i++ {
			logs += fmt.Sprintf("step %d\n", i)
			h.db.Model(&models.Job{}).Where("id = ?", j.ID).Update("logs", logs)
			time.Sleep(15 * time.Millisecond)
		}
		now := time.Now()
		h.db.Model(&models.Job{}).Where("id = ?", j.ID).Updates(map[string]any{"status": models.JobStatusFailed, "completed_at": now, "logs": logs + "boom\n"})
	}()
	var got strings.Builder
	final, err := h.svc.FollowLogs(ctx, j.ID, func(chunk string) error {
		got.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "step 1\nstep 2\nstep 3\nstep 4\nstep 5\nboom\n"
	if got.String() != want || final.Status != models.JobStatusFailed {
		t.Fatalf("followed %q (%s), want %q (failed)", got.String(), final.Status, want)
	}

	// Finished job: one chunk, then done.
	var chunks []string
	if _, err := h.svc.FollowLogs(ctx, j.ID, func(c string) error { chunks = append(chunks, c); return nil }); err != nil || len(chunks) != 1 {
		t.Fatalf("finished follow = %v, %v", chunks, err)
	}

	// Reader going away stops the follow.
	r := h.job(t, p, models.JobTypeInstallVersion, models.JobStatusRunning, time.Now(), nil)
	h.db.Model(&models.Job{}).Where("id = ?", r.ID).Update("logs", "x\n")
	if _, err := h.svc.FollowLogs(ctx, r.ID, func(string) error { return io.ErrClosedPipe }); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("closed reader = %v", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := h.svc.FollowLogs(cctx, r.ID, func(string) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx end = %v", err)
	}
}

// TestInlineJobCancelledFromAnotherProcess: a CLI inline job flushes its
// log while running and stops when its row is cancelled (Jobs page or
// "nebi job cancel" in another terminal).
func TestInlineJobCancelledFromAnotherProcess(t *testing.T) {
	h := newHarness(t, false)
	ctx := context.Background()
	p := h.project(t, "p")

	type result struct {
		job *models.Job
		err error
	}
	res := make(chan result, 1)
	go func() {
		j, err := h.svc.Submit(ctx, shared.JobRequest{ProjectID: p.ID, Type: testBlockJob})
		res <- result{j, err}
	}()

	var running models.Job
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.db.Where("type = ?", testBlockJob).First(&running)
		if running.Status == models.JobStatusRunning && strings.Contains(running.Logs, "waiting") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("inline job not running with flushed log: %+v", running)
		}
		time.Sleep(10 * time.Millisecond)
	}

	other := New(h.sdk, Options{}) // a second process with no worker
	if _, err := other.Cancel(ctx, running.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-res:
		if r.job.Status != models.JobStatusCancelled || r.err == nil {
			t.Fatalf("inline job = %s, %v; want cancelled with an error", r.job.Status, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("inline job did not stop")
	}
	d := h.waitStatus(t, running.ID, models.JobStatusCancelled)
	if !strings.Contains(d.Logs, "waiting") {
		t.Fatalf("final log %q", d.Logs)
	}
}

func TestInlineJobOutcomes(t *testing.T) {
	h := newHarness(t, false)
	ctx := context.Background()
	p := h.project(t, "p")
	j, err := h.svc.Submit(ctx, shared.JobRequest{ProjectID: p.ID, Type: testLinesJob})
	if err != nil || j.Status != models.JobStatusCompleted || !strings.Contains(j.Logs, "line 20") {
		t.Fatalf("inline success = %+v, %v", j, err)
	}

	cctx, cancel := context.WithCancel(ctx)
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	j, err = h.svc.Submit(cctx, shared.JobRequest{ProjectID: p.ID, Type: testBlockJob})
	if err == nil || j.Status != models.JobStatusCancelled {
		t.Fatalf("inline Ctrl-C = %+v, %v", j, err)
	}
	var stored models.Job
	h.db.First(&stored, "id = ?", j.ID)
	if stored.Status != models.JobStatusCancelled {
		t.Fatalf("stored status %q", stored.Status)
	}
}

func TestFailedInstallLinkage(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)

	failedLast := h.project(t, "failed-last")
	h.job(t, failedLast, models.JobTypeInstallVersion, models.JobStatusCompleted, base, nil)
	want := h.job(t, failedLast, models.JobTypeInstallVersion, models.JobStatusFailed, base.Add(time.Minute), nil)
	// Later non-install jobs and cancelled installs do not clear it.
	h.job(t, failedLast, models.JobTypeCreateVersion, models.JobStatusCompleted, base.Add(2*time.Minute), nil)
	h.job(t, failedLast, models.JobTypeInstallVersion, models.JobStatusCancelled, base.Add(3*time.Minute), nil)

	recovered := h.project(t, "recovered")
	h.job(t, recovered, models.JobTypeInstallVersion, models.JobStatusFailed, base, nil)
	h.job(t, recovered, models.JobTypeUninstall, models.JobStatusCompleted, base.Add(time.Minute), nil)

	createInstall := h.project(t, "create-install")
	ci := h.job(t, createInstall, models.JobTypeCreate, models.JobStatusFailed, base, map[string]any{"install": true})

	createOnly := h.project(t, "create-only")
	h.job(t, createOnly, models.JobTypeCreate, models.JobStatusFailed, base, map[string]any{"install": false})

	// A create that failed before any version existed: sdk/local reports
	// not_installed, so the link stays null even though the job counts.
	createEarly := h.project(t, "create-early")
	h.job(t, createEarly, models.JobTypeCreate, models.JobStatusFailed, base, map[string]any{"install": true})

	// InstallStatus as sdk/local computes it before the decorators run.
	failed := models.InstallStatusFailed
	items := []shared.ProjectListItem{
		{ID: failedLast.ID, InstallStatus: failed},
		{ID: recovered.ID, InstallStatus: models.InstallStatusNotInstalled},
		{ID: createInstall.ID, InstallStatus: failed},
		{ID: createOnly.ID, InstallStatus: models.InstallStatusNotInstalled},
		{ID: createEarly.ID, InstallStatus: models.InstallStatusNotInstalled},
	}
	if err := h.svc.DecorateProjects(ctx, items); err != nil {
		t.Fatal(err)
	}
	if items[4].FailedInstallJobID != nil {
		t.Fatalf("create failed before a version = %v, want nil", items[4].FailedInstallJobID)
	}
	if items[0].FailedInstallJobID == nil || *items[0].FailedInstallJobID != want.ID {
		t.Fatalf("failed-last = %v, want %s", items[0].FailedInstallJobID, want.ID)
	}
	if items[1].FailedInstallJobID != nil {
		t.Fatalf("recovered = %v, want nil", items[1].FailedInstallJobID)
	}
	if items[2].FailedInstallJobID == nil || *items[2].FailedInstallJobID != ci.ID {
		t.Fatalf("create-and-install = %v, want %s", items[2].FailedInstallJobID, ci.ID)
	}
	if items[3].FailedInstallJobID != nil {
		t.Fatalf("create without install = %v, want nil", items[3].FailedInstallJobID)
	}
	if err := h.svc.DecorateProjects(ctx, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitNeedsProject(t *testing.T) {
	h := newHarness(t, true)
	var verr *shared.ValidationError
	if _, err := h.svc.Submit(context.Background(), shared.JobRequest{Type: testLinesJob}); !errors.As(err, &verr) {
		t.Fatalf("Submit without project = %v", err)
	}
}
