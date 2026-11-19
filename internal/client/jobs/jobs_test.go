package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// timeout bounds every wait in these tests, so a broken runner fails the
// test instead of hanging it. Nothing here sleeps to let things happen.
const timeout = 10 * time.Second

func newRunner(t *testing.T, opts Options) *Memory {
	t.Helper()
	m := NewMemory(opts)
	t.Cleanup(m.Close)
	return m
}

func waitCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func noop(context.Context, io.Writer) error { return nil }

func mustSubmit(t *testing.T, m *Memory, projectID uuid.UUID, typ Type, run Func) *Job {
	t.Helper()
	j, err := m.Submit(context.Background(), projectID, typ, run)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return j
}

func mustWait(t *testing.T, m *Memory, id uuid.UUID) Job {
	t.Helper()
	j, err := m.Wait(waitCtx(t), id)
	if err != nil {
		t.Fatalf("Wait(%s): %v", id, err)
	}
	return j
}

func mustGet(t *testing.T, m *Memory, id uuid.UUID) Job {
	t.Helper()
	j, err := m.Get(id)
	if err != nil {
		t.Fatalf("Get(%s): %v", id, err)
	}
	return j
}

func wantStatus(t *testing.T, m *Memory, id uuid.UUID, want Status) {
	t.Helper()
	if got := mustGet(t, m, id); got.Status != want {
		t.Fatalf("job %s: status %s, want %s (%+v)", id, got.Status, want, got)
	}
}

// gate is a job body that reports when it starts and returns nil when
// released, or the context's error when the job is cancelled.
type gate struct {
	started chan struct{}
	release chan struct{}
	ctxErr  atomic.Value // the context error seen when the body returned
}

func newGate() *gate {
	return &gate{started: make(chan struct{}), release: make(chan struct{})}
}

func (g *gate) body(ctx context.Context, _ io.Writer) error {
	close(g.started)
	select {
	case <-g.release:
		g.ctxErr.Store(fmt.Sprint(ctx.Err()))
		return nil
	case <-ctx.Done():
		g.ctxErr.Store(fmt.Sprint(ctx.Err()))
		return fmt.Errorf("stopped: %w", ctx.Err())
	}
}

func (g *gate) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-g.started:
	case <-time.After(timeout):
		t.Fatal("job body did not start")
	}
}

func (g *gate) open() { close(g.release) }

func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatal("timed out waiting on a channel")
		panic("unreachable")
	}
}

func send(t *testing.T, ch chan<- struct{}) {
	t.Helper()
	select {
	case ch <- struct{}{}:
	case <-time.After(timeout):
		t.Fatal("timed out sending on a channel")
	}
}

// counts returns how many of the jobs are running and pending.
func counts(t *testing.T, m *Memory, ids []uuid.UUID) (running, pending int) {
	t.Helper()
	for _, id := range ids {
		switch mustGet(t, m, id).Status {
		case StatusRunning:
			running++
		case StatusPending:
			pending++
		}
	}
	return running, pending
}

// --- Status names -------------------------------------------------------

// TestStatusNames pins the status values to those of the server's job
// model (internal/models.JobStatus) and the frontend. internal/client must
// not import internal/models, so the values are spelled out here.
func TestStatusNames(t *testing.T) {
	for status, want := range map[Status]string{
		StatusPending:   "pending",
		StatusRunning:   "running",
		StatusCompleted: "completed",
		StatusFailed:    "failed",
		StatusCancelled: "cancelled",
	} {
		if string(status) != want {
			t.Errorf("status %q, want %q", status, want)
		}
	}
	for _, s := range []Status{StatusPending, StatusRunning} {
		if !(Job{Status: s}).Active() {
			t.Errorf("%s must be active", s)
		}
	}
	for _, s := range []Status{StatusCompleted, StatusFailed, StatusCancelled} {
		if (Job{Status: s}).Active() {
			t.Errorf("%s must not be active", s)
		}
	}
}

// --- Basics -------------------------------------------------------------

func TestSubmitSuccessAndFailure(t *testing.T) {
	m := newRunner(t, Options{})
	project := uuid.New()

	ok := mustSubmit(t, m, project, TypeCreate, noop)
	if ok.ProjectID != project || ok.Type != TypeCreate || !ok.Active() {
		t.Fatalf("unexpected submitted job: %+v", ok)
	}
	bad := mustSubmit(t, m, project, TypeInstall, func(context.Context, io.Writer) error { return errors.New("boom") })

	got := mustWait(t, m, ok.ID)
	if got.Status != StatusCompleted || got.Error != "" || got.StartedAt == nil || got.FinishedAt == nil || got.Active() {
		t.Fatalf("want completed job, got %+v", got)
	}
	got = mustWait(t, m, bad.ID)
	if got.Status != StatusFailed || got.Error != "boom" {
		t.Fatalf("want failed job with error, got %+v", got)
	}
}

func TestPanicBecomesFailure(t *testing.T) {
	m := newRunner(t, Options{})
	j := mustSubmit(t, m, uuid.New(), TypeCreate, func(context.Context, io.Writer) error { panic("oops") })
	if got := mustWait(t, m, j.ID); got.Status != StatusFailed || got.Error != "job panicked: oops" {
		t.Fatalf("got %+v", got)
	}
	// The slot and the project are free again.
	if got := mustWait(t, m, mustSubmit(t, m, j.ProjectID, TypeInstall, noop).ID); got.Status != StatusCompleted {
		t.Fatalf("job after a panic: %+v", got)
	}
}

func TestProjectJobsInSubmissionOrder(t *testing.T) {
	m := newRunner(t, Options{})
	project, other := uuid.New(), uuid.New()
	g := newGate()
	first := mustSubmit(t, m, project, TypeCreate, g.body)
	mustSubmit(t, m, other, TypeCreate, noop)
	second := mustSubmit(t, m, project, TypeDeleteFiles, noop)
	g.waitStarted(t)

	list, err := m.ProjectJobs(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("want [first, second] for the project in submission order, got %+v", list)
	}
	g.open()
	mustWait(t, m, second.ID)
}

func TestContextCarriesJobIDAndStage(t *testing.T) {
	m := newRunner(t, Options{})
	var seen uuid.UUID
	version := uuid.New()
	j := mustSubmit(t, m, uuid.New(), TypeCreate, func(ctx context.Context, _ io.Writer) error {
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
	got := mustWait(t, m, j.ID)
	if got.Status != StatusCompleted {
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

func TestUnknownJobAndNilBody(t *testing.T) {
	m := newRunner(t, Options{})
	id := uuid.New()
	if _, err := m.Get(id); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Get: want ErrUnknownJob, got %v", err)
	}
	if _, err := m.Wait(waitCtx(t), id); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Wait: want ErrUnknownJob, got %v", err)
	}
	if err := m.Cancel(context.Background(), id); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Cancel: want ErrUnknownJob, got %v", err)
	}
	if _, err := m.Logs(id); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Logs: want ErrUnknownJob, got %v", err)
	}
	if err := m.FollowLogs(waitCtx(t), id, io.Discard); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("FollowLogs: want ErrUnknownJob, got %v", err)
	}
	if _, err := m.Submit(context.Background(), uuid.New(), TypeCreate, nil); err == nil {
		t.Fatal("Submit with nil body should fail")
	}
}

// --- Concurrency --------------------------------------------------------

func TestDefaultOptions(t *testing.T) {
	for _, opts := range []Options{{}, {MaxRunning: -1, KeepFinished: -1, LogLimit: -1}} {
		m := NewMemory(opts)
		if m.maxRunning != 2 || m.keepFinished != 200 || m.logLimit != 1<<20 {
			t.Errorf("NewMemory(%+v): maxRunning %d, keepFinished %d, logLimit %d; want 2, 200, 1 MiB",
				opts, m.maxRunning, m.keepFinished, m.logLimit)
		}
		m.Close()
	}
}

func TestDefaultLimitIsTwo(t *testing.T) {
	m := newRunner(t, Options{})
	a, b := newGate(), newGate()
	ja := mustSubmit(t, m, uuid.New(), TypeCreate, a.body)
	jb := mustSubmit(t, m, uuid.New(), TypeCreate, b.body)
	jc := mustSubmit(t, m, uuid.New(), TypeCreate, noop)
	if jc.Status != StatusPending {
		t.Fatalf("a third job must wait with the default limit of 2, got %+v", jc)
	}
	a.waitStarted(t)
	b.waitStarted(t)
	wantStatus(t, m, ja.ID, StatusRunning)
	wantStatus(t, m, jb.ID, StatusRunning)
	wantStatus(t, m, jc.ID, StatusPending)
	a.open()
	if got := mustWait(t, m, jc.ID); got.Status != StatusCompleted {
		t.Fatalf("third job: %+v", got)
	}
	b.open()
}

func TestGlobalLimit(t *testing.T) {
	const limit, total = 3, 10
	m := newRunner(t, Options{MaxRunning: limit})

	var cur, peak atomic.Int32
	started := make(chan int, total)
	release := make(chan struct{})
	ids := make([]uuid.UUID, total)
	for i := range total {
		j := mustSubmit(t, m, uuid.New(), TypeInstall, func(ctx context.Context, _ io.Writer) error {
			n := cur.Add(1)
			for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
			}
			started <- i
			defer cur.Add(-1)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		})
		ids[i] = j.ID
	}

	for range limit {
		recv(t, started)
	}
	if running, pending := counts(t, m, ids); running != limit || pending != total-limit {
		t.Fatalf("with %d slots: %d running, %d pending; want %d and %d", limit, running, pending, limit, total-limit)
	}
	for done := 0; done < total; done++ {
		send(t, release) // one body returns
		if done+limit < total {
			recv(t, started) // and exactly one more starts
		}
		if running, _ := counts(t, m, ids); running > limit {
			t.Fatalf("%d jobs running, limit %d", running, limit)
		}
	}
	for _, id := range ids {
		if got := mustWait(t, m, id); got.Status != StatusCompleted {
			t.Fatalf("job %s: %+v", id, got)
		}
	}
	if p := peak.Load(); p > limit {
		t.Fatalf("%d job bodies ran at once, limit %d", p, limit)
	}
}

func TestOneJobPerProject(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 4})
	project, other := uuid.New(), uuid.New()

	var inProject atomic.Int32
	var overlap atomic.Bool
	gates := []*gate{newGate(), newGate(), newGate()}
	ids := make([]uuid.UUID, len(gates))
	for i, g := range gates {
		ids[i] = mustSubmit(t, m, project, TypeInstall, func(ctx context.Context, w io.Writer) error {
			if inProject.Add(1) > 1 {
				overlap.Store(true)
			}
			defer inProject.Add(-1)
			return g.body(ctx, w)
		}).ID
	}
	gates[0].waitStarted(t)
	wantStatus(t, m, ids[1], StatusPending)
	wantStatus(t, m, ids[2], StatusPending)

	// Another project runs at once while this project's jobs wait.
	o := newGate()
	oj := mustSubmit(t, m, other, TypeInstall, o.body)
	o.waitStarted(t)
	wantStatus(t, m, oj.ID, StatusRunning)

	for i, g := range gates {
		if i > 0 {
			g.waitStarted(t)
		}
		wantStatus(t, m, ids[i], StatusRunning)
		for _, later := range ids[i+1:] {
			wantStatus(t, m, later, StatusPending)
		}
		g.open()
		mustWait(t, m, ids[i])
	}
	o.open()
	mustWait(t, m, oj.ID)
	if overlap.Load() {
		t.Fatal("two jobs of one project ran at the same time")
	}
}

func TestPendingJobsStartInSubmissionOrder(t *testing.T) {
	const total = 6
	m := newRunner(t, Options{MaxRunning: 1})
	order := make(chan int, total)
	gates := make([]*gate, total)
	ids := make([]uuid.UUID, total)
	for i := range total {
		gates[i] = newGate()
		g := gates[i]
		ids[i] = mustSubmit(t, m, uuid.New(), TypeInstall, func(ctx context.Context, w io.Writer) error {
			order <- i
			return g.body(ctx, w)
		}).ID
	}
	for want := range total {
		if got := recv(t, order); got != want {
			t.Fatalf("job %d started, want job %d", got, want)
		}
		if running, pending := counts(t, m, ids); running != 1 || pending != total-want-1 {
			t.Fatalf("after job %d started: %d running, %d pending", want, running, pending)
		}
		gates[want].open()
	}
}

func TestBusyProjectDoesNotHoldBackOthers(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 2})
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	a1g, b1g := newGate(), newGate()
	a1 := mustSubmit(t, m, a, TypeInstall, a1g.body)
	a2 := mustSubmit(t, m, a, TypeInstall, noop) // waits for project a
	b1 := mustSubmit(t, m, b, TypeInstall, b1g.body)
	c1 := mustSubmit(t, m, c, TypeInstall, noop) // waits for a slot
	a1g.waitStarted(t)
	b1g.waitStarted(t)
	wantStatus(t, m, a1.ID, StatusRunning)
	wantStatus(t, m, b1.ID, StatusRunning)
	wantStatus(t, m, a2.ID, StatusPending)
	wantStatus(t, m, c1.ID, StatusPending)

	// A slot frees: a2 is older, but its project is busy, so c1 starts.
	b1g.open()
	if got := mustWait(t, m, c1.ID); got.Status != StatusCompleted {
		t.Fatalf("c1: %+v", got)
	}
	wantStatus(t, m, a2.ID, StatusPending)

	a1g.open()
	if got := mustWait(t, m, a2.ID); got.Status != StatusCompleted {
		t.Fatalf("a2: %+v", got)
	}
	if a2s, c1s := mustGet(t, m, a2.ID), mustGet(t, m, c1.ID); a2s.StartedAt.Before(*c1s.StartedAt) {
		t.Fatal("c1 must have started before a2")
	}
}

func TestPendingJobIsActive(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 1})
	a, b := uuid.New(), uuid.New()
	g := newGate()
	a1 := mustSubmit(t, m, a, TypeInstall, g.body)
	b1 := mustSubmit(t, m, b, TypeInstall, noop)
	b2 := mustSubmit(t, m, b, TypeUninstall, noop)
	g.waitStarted(t)
	wantStatus(t, m, a1.ID, StatusRunning)
	if b1.Status != StatusPending || !b1.Active() {
		t.Fatalf("Submit must return a pending, active job when no slot is free: %+v", b1)
	}

	active, err := m.RetireProject(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || active[0] != b1.ID || active[1] != b2.ID {
		t.Fatalf("pending jobs must keep a project busy: active = %v, want [%s %s]", active, b1.ID, b2.ID)
	}
	if _, err := m.Submit(context.Background(), b, TypeInstall, noop); err != nil {
		t.Fatalf("a refused retire must not block jobs: %v", err)
	}
	g.open()
	for _, j := range append(projectJobs(t, m, a), projectJobs(t, m, b)...) {
		mustWait(t, m, j.ID)
	}
}

// --- Retire -------------------------------------------------------------

func TestRetireProject(t *testing.T) {
	m := newRunner(t, Options{})
	ctx := context.Background()
	project, other := uuid.New(), uuid.New()

	g := newGate()
	busy := mustSubmit(t, m, project, TypeInstall, g.body)
	active, err := m.RetireProject(ctx, project, TypeDeleteFiles)
	if err != nil || len(active) != 1 || active[0] != busy.ID {
		t.Fatalf("busy project: active = %v, %v; want [%s]", active, err, busy.ID)
	}
	g.open()
	mustWait(t, m, busy.ID)

	if active, err := m.RetireProject(ctx, project, TypeDeleteFiles); err != nil || len(active) != 0 {
		t.Fatalf("idle project: active = %v, %v", active, err)
	}
	if _, err := m.Submit(ctx, project, TypeInstall, noop); !errors.Is(err, ErrProjectRetired) {
		t.Fatalf("retired project: want ErrProjectRetired, got %v", err)
	}
	if _, err := m.Submit(ctx, other, TypeInstall, noop); err != nil {
		t.Fatalf("other projects are not affected: %v", err)
	}
	if err := m.RestoreProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	mustWait(t, m, mustSubmit(t, m, project, TypeInstall, noop).ID)
}

func TestRetirementEndsWhenRemovalCompletes(t *testing.T) {
	m := newRunner(t, Options{})
	ctx := context.Background()

	removed := uuid.New()
	if _, err := m.RetireProject(ctx, removed, TypeDeleteFiles); err != nil {
		t.Fatal(err)
	}
	g := newGate()
	removal := mustSubmit(t, m, removed, TypeDeleteFiles, g.body)
	g.waitStarted(t)
	if _, err := m.Submit(ctx, removed, TypeInstall, noop); !errors.Is(err, ErrProjectRetired) {
		t.Fatalf("while the removal runs the project stays retired, got %v", err)
	}
	g.open()
	if got := mustWait(t, m, removal.ID); got.Status != StatusCompleted {
		t.Fatalf("removal: %+v", got)
	}
	if retiredCount(m) != 0 {
		t.Fatal("the retired entry must be dropped when the removal job completes")
	}

	stuck := uuid.New()
	if _, err := m.RetireProject(ctx, stuck, TypeDeleteFiles); err != nil {
		t.Fatal(err)
	}
	failed := mustSubmit(t, m, stuck, TypeDeleteFiles, func(context.Context, io.Writer) error { return errors.New("permission denied") })
	if got := mustWait(t, m, failed.ID); got.Status != StatusFailed {
		t.Fatalf("failed removal: %+v", got)
	}
	if _, err := m.Submit(ctx, stuck, TypeInstall, noop); !errors.Is(err, ErrProjectRetired) {
		t.Fatalf("a failed removal must keep the project retired, got %v", err)
	}
	if retiredCount(m) != 1 {
		t.Fatal("the retired entry must stay when the removal job fails")
	}
}

// --- Eviction -----------------------------------------------------------

func TestEvictionKeepsActiveAndLatestPerProject(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 4, KeepFinished: 3})
	z, w, x, y := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	zg := newGate()
	zj := mustSubmit(t, m, z, TypeInstall, zg.body) // stays active throughout
	w1 := mustSubmit(t, m, w, TypeCreate, func(context.Context, io.Writer) error { return errors.New("lock failed") })
	mustWait(t, m, w1.ID)
	wg := newGate()
	w2 := mustSubmit(t, m, w, TypeInstall, wg.body) // w1 stays w's latest finished job
	x1 := mustSubmit(t, m, x, TypeCreate, noop)
	mustWait(t, m, x1.ID)
	var ys []uuid.UUID
	for range 10 {
		j := mustSubmit(t, m, y, TypeInstall, noop)
		mustWait(t, m, j.ID)
		ys = append(ys, j.ID)
	}

	wantStatus(t, m, zj.ID, StatusRunning)
	wantStatus(t, m, w2.ID, StatusRunning)
	wantStatus(t, m, w1.ID, StatusFailed) // w's latest finished job
	wantStatus(t, m, x1.ID, StatusCompleted)
	// y keeps its latest job; the 3 most recently finished others are y7..y9.
	if got := ids(projectJobs(t, m, y)); !equalIDs(got, ys[6:]) {
		t.Fatalf("project y keeps %v, want %v", got, ys[6:])
	}
	for _, id := range ys[:6] {
		if _, err := m.Get(id); !errors.Is(err, ErrUnknownJob) {
			t.Fatalf("old finished job %s must be dropped, got %v", id, err)
		}
	}

	// Once w2 finishes, w1 is still w's latest create job, so it stays
	// next to w2, the latest install.
	zg.open()
	wg.open()
	mustWait(t, m, zj.ID)
	mustWait(t, m, w2.ID)
	for _, id := range append([]uuid.UUID{zj.ID, w1.ID, w2.ID, x1.ID}, ys[6:]...) {
		mustGet(t, m, id)
	}
	if n := jobCount(m); n != 8 {
		t.Fatalf("%d jobs kept, want 8 (latest per project and type: z, w create, w install, x, y10; plus 3 others)", n)
	}
}

func TestEvictionKeepsLatestJobOfEachType(t *testing.T) {
	m := newRunner(t, Options{KeepFinished: 1})
	p := uuid.New()
	failed := mustSubmit(t, m, p, TypeInstall, func(context.Context, io.Writer) error { return errors.New("solve failed") })
	mustWait(t, m, failed.ID)
	later := mustSubmit(t, m, p, TypeUninstall, noop) // a later job of another type
	mustWait(t, m, later.ID)
	oldInstall := mustSubmit(t, m, p, TypeInstall, noop)
	mustWait(t, m, oldInstall.ID)
	newInstall := mustSubmit(t, m, p, TypeInstall, func(context.Context, io.Writer) error { return errors.New("solve failed again") })
	mustWait(t, m, newInstall.ID)

	// Unkept jobs of other projects push p's older jobs out of the tail.
	for range 3 {
		mustWait(t, m, mustSubmit(t, m, uuid.New(), TypeDeleteFiles, noop).ID)
	}
	// p keeps its latest uninstall and its latest install; the earlier
	// installs are ordinary old jobs and age out.
	if got, want := ids(projectJobs(t, m, p)), []uuid.UUID{later.ID, newInstall.ID}; !equalIDs(got, want) {
		t.Fatalf("project keeps %v, want its latest uninstall and install %v", got, want)
	}
}

func TestEvictionSkipsJobsCancelledWhilePending(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 1, KeepFinished: 1})
	p := uuid.New()
	failed := mustSubmit(t, m, p, TypeInstall, func(context.Context, io.Writer) error { return errors.New("partial env") })
	mustWait(t, m, failed.ID)

	// A later install waits behind another project's job and is cancelled
	// before it starts.
	g := newGate()
	blocker := mustSubmit(t, m, uuid.New(), TypeCreate, g.body)
	g.waitStarted(t)
	never := mustSubmit(t, m, p, TypeInstall, noop)
	if err := m.Cancel(context.Background(), never.ID); err != nil {
		t.Fatal(err)
	}
	g.open()
	mustWait(t, m, blocker.ID)

	// Unkept jobs of other projects push the tail out.
	for range 3 {
		mustWait(t, m, mustSubmit(t, m, uuid.New(), TypeDeleteFiles, noop).ID)
	}
	if _, err := m.Get(failed.ID); err != nil {
		t.Fatalf("the latest install that ran must stay kept, got %v", err)
	}
	if _, err := m.Get(never.ID); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("an install cancelled while pending must age out like any old job, got %v", err)
	}
}

func TestEvictionDefaultCap(t *testing.T) {
	m := newRunner(t, Options{})
	project := uuid.New()
	var all []uuid.UUID
	for range 260 {
		j := mustSubmit(t, m, project, TypeInstall, noop)
		mustWait(t, m, j.ID)
		all = append(all, j.ID)
	}
	// The latest job, plus the 200 most recently finished others.
	if got := ids(projectJobs(t, m, project)); !equalIDs(got, all[59:]) {
		t.Fatalf("kept %d jobs from %s, want the last 201 (from %s)", len(got), got[0], all[59])
	}
}

func TestCreateDeleteCyclesStayBounded(t *testing.T) {
	m := newRunner(t, Options{})
	ctx := context.Background()
	for range 500 {
		p := uuid.New()
		mustWait(t, m, mustSubmit(t, m, p, TypeCreate, noop).ID)
		if active, err := m.RetireProject(ctx, p, TypeDeleteFiles); err != nil || len(active) != 0 {
			t.Fatalf("RetireProject: %v, %v", active, err)
		}
		mustWait(t, m, mustSubmit(t, m, p, TypeDeleteFiles, noop).ID)
	}
	if n := jobCount(m); n != 200 {
		t.Fatalf("%d jobs kept after 500 create and delete cycles, want 200", n)
	}
	if n := retiredCount(m); n != 0 {
		t.Fatalf("%d retired entries kept after their removals completed, want 0", n)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.finished) != 200 || len(m.byProj) != 100 || len(m.pending) != 0 || m.running != 0 || len(m.busy) != 0 {
		t.Fatalf("internal state not cleaned up: finished %d, projects %d, pending %d, running %d, busy %d",
			len(m.finished), len(m.byProj), len(m.pending), m.running, len(m.busy))
	}
}

func TestRetiredProjectsLatestJobIsNotKept(t *testing.T) {
	m := newRunner(t, Options{KeepFinished: 1})
	ctx := context.Background()
	deleted := uuid.New()
	old := mustSubmit(t, m, deleted, TypeCreate, noop)
	mustWait(t, m, old.ID)
	if _, err := m.RetireProject(ctx, deleted); err != nil { // deleted without its files
		t.Fatal(err)
	}
	for range 2 {
		mustWait(t, m, mustSubmit(t, m, uuid.New(), TypeDeleteFiles, noop).ID)
	}
	if _, err := m.Get(old.ID); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("a deleted project's last job must age out like any other, got %v", err)
	}
}

// --- Cancel -------------------------------------------------------------

func TestCancelPendingJob(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 1})
	g := newGate()
	blocker := mustSubmit(t, m, uuid.New(), TypeInstall, g.body)
	var ran atomic.Bool
	project := uuid.New()
	pending := mustSubmit(t, m, project, TypeCreate, func(context.Context, io.Writer) error { ran.Store(true); return nil })
	g.waitStarted(t)

	if err := m.Cancel(context.Background(), pending.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got := mustWait(t, m, pending.ID) // returns at once
	if got.Status != StatusCancelled || got.StartedAt != nil || got.FinishedAt == nil || got.Error != "" {
		t.Fatalf("want cancelled without starting, got %+v", got)
	}
	if active, _ := m.RetireProject(context.Background(), project); len(active) != 0 {
		t.Fatalf("a cancelled job must not keep its project busy: %v", active)
	}
	_ = m.RestoreProject(context.Background(), project)

	// The freed slot goes to later jobs; the cancelled one never runs.
	g.open()
	mustWait(t, m, blocker.ID)
	mustWait(t, m, mustSubmit(t, m, project, TypeInstall, noop).ID)
	if ran.Load() {
		t.Fatal("a job cancelled while pending must never run")
	}
	wantStatus(t, m, pending.ID, StatusCancelled)
}

func TestCancelRunningJob(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 1})
	g := newGate()
	j := mustSubmit(t, m, uuid.New(), TypeCreate, g.body)
	next := mustSubmit(t, m, uuid.New(), TypeCreate, noop)
	g.waitStarted(t)

	if err := m.Cancel(context.Background(), j.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got := mustWait(t, m, j.ID)
	if got.Status != StatusCancelled || got.Error != "" || got.StartedAt == nil || got.FinishedAt == nil {
		t.Fatalf("want cancelled (not failed) although the body returned an error, got %+v", got)
	}
	if e := g.ctxErr.Load(); e != context.Canceled.Error() {
		t.Fatalf("the body's context must be cancelled, saw %v", e)
	}
	if got := mustWait(t, m, next.ID); got.Status != StatusCompleted {
		t.Fatalf("the slot must be freed: %+v", got)
	}
}

func TestCancelledBodyThatSucceedsIsCompleted(t *testing.T) {
	m := newRunner(t, Options{})
	started := make(chan struct{})
	j := mustSubmit(t, m, uuid.New(), TypeCreate, func(ctx context.Context, _ io.Writer) error {
		close(started)
		<-ctx.Done()
		return nil // the work was done anyway
	})
	recv(t, started)
	if err := m.Cancel(context.Background(), j.ID); err != nil {
		t.Fatal(err)
	}
	if got := mustWait(t, m, j.ID); got.Status != StatusCompleted {
		t.Fatalf("a body that returns nil completed, got %+v", got)
	}
}

func TestFailureBeforeCancelStaysFailed(t *testing.T) {
	m := newRunner(t, Options{})
	proceed := make(chan struct{})
	jobCtx := make(chan context.Context, 1)
	j := mustSubmit(t, m, uuid.New(), TypeCreate, func(ctx context.Context, _ io.Writer) error {
		jobCtx <- ctx
		<-proceed
		return errors.New("lock failed") // fails on its own, not cancelled
	})
	ctx := recv(t, jobCtx)

	// Hold the runner's lock so the body returns but the runner cannot yet
	// record it. The runner cancels the body's context itself once the
	// body has returned, so ctx being done means it has.
	m.mu.Lock()
	close(proceed)
	select {
	case <-ctx.Done():
	case <-time.After(timeout):
		m.mu.Unlock()
		t.Fatal("body did not return")
	}
	// Now cancel, before the runner records the failure.
	err := m.cancelLocked(m.jobs[j.ID])
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if got := mustWait(t, m, j.ID); got.Status != StatusFailed || got.Error != "lock failed" {
		t.Fatalf("a body that failed before the cancel reached it must stay failed, got %+v", got)
	}
}

func TestCancelFinishedJobIsNoop(t *testing.T) {
	m := newRunner(t, Options{})
	for _, body := range []Func{noop, func(context.Context, io.Writer) error { return errors.New("boom") }} {
		j := mustSubmit(t, m, uuid.New(), TypeInstall, body)
		before := mustWait(t, m, j.ID)
		if err := m.Cancel(context.Background(), j.ID); err != nil {
			t.Fatalf("Cancel of a finished job: %v", err)
		}
		if after := mustGet(t, m, j.ID); after.Status != before.Status || after.Error != before.Error || !after.FinishedAt.Equal(*before.FinishedAt) {
			t.Fatalf("a finished job must be left unchanged: before %+v, after %+v", before, after)
		}
	}
}

func TestRemovalJobCannotBeCancelled(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 1})
	running := newGate()
	r1 := mustSubmit(t, m, uuid.New(), TypeDeleteFiles, running.body)
	r2 := mustSubmit(t, m, uuid.New(), TypeDeleteFiles, noop) // pending
	running.waitStarted(t)

	for _, j := range []*Job{r1, r2} {
		err := m.Cancel(context.Background(), j.ID)
		var nc *NotCancellableError
		if !errors.As(err, &nc) || !errors.Is(err, ErrNotCancellable) || nc.JobID != j.ID || nc.Type != TypeDeleteFiles {
			t.Fatalf("Cancel of a removal job: want *NotCancellableError, got %#v", err)
		}
	}
	wantStatus(t, m, r1.ID, StatusRunning)
	wantStatus(t, m, r2.ID, StatusPending)

	running.open()
	for _, j := range []*Job{r1, r2} {
		if got := mustWait(t, m, j.ID); got.Status != StatusCompleted {
			t.Fatalf("removal job must run to the end: %+v", got)
		}
	}
	if e := running.ctxErr.Load(); e != "<nil>" {
		t.Fatalf("a removal job's context must not be cancelled, saw %v", e)
	}
	if err := m.Cancel(context.Background(), r1.ID); err != nil {
		t.Fatalf("cancelling a finished removal job does nothing: %v", err)
	}
	if TypeDeleteFiles.Cancellable() || !TypeCreate.Cancellable() || !TypeInstall.Cancellable() || !TypeUninstall.Cancellable() {
		t.Fatal("only file removal jobs are not cancellable")
	}
}

// --- Close --------------------------------------------------------------

func TestCloseCancelsAndRejects(t *testing.T) {
	m := NewMemory(Options{MaxRunning: 1})
	g := newGate()
	running := mustSubmit(t, m, uuid.New(), TypeCreate, g.body)
	var ran atomic.Bool
	pending := mustSubmit(t, m, uuid.New(), TypeCreate, func(context.Context, io.Writer) error { ran.Store(true); return nil })
	g.waitStarted(t)
	m.Close()

	if got := mustGet(t, m, running.ID); got.Status != StatusCancelled {
		t.Fatalf("a running job stopped by Close is cancelled, got %+v", got)
	}
	if got := mustGet(t, m, pending.ID); got.Status != StatusCancelled || got.StartedAt != nil || ran.Load() {
		t.Fatalf("a pending job is cancelled by Close without running, got %+v", got)
	}
	if _, err := m.Submit(context.Background(), uuid.New(), TypeCreate, noop); !errors.Is(err, ErrClosed) {
		t.Fatalf("Submit after Close: want ErrClosed, got %v", err)
	}
	m.Close() // a second Close is harmless
}

// waitClosed waits until Close has stopped accepting jobs.
func waitClosed(t *testing.T, m *Memory) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		_, err := m.Submit(context.Background(), uuid.New(), TypeCreate, noop)
		if errors.Is(err, ErrClosed) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Close did not start")
		}
		runtime.Gosched()
	}
}

func TestCloseLetsRemovalJobsFinish(t *testing.T) {
	m := NewMemory(Options{MaxRunning: 2})
	release := make(chan struct{})
	removal := func(err error) *Job {
		started := make(chan struct{})
		j := mustSubmit(t, m, uuid.New(), TypeDeleteFiles, func(context.Context, io.Writer) error {
			close(started)
			<-release // a removal does not stop part way
			return err
		})
		recv(t, started)
		return j
	}
	failed := removal(errors.New("permission denied"))
	completed := removal(nil)

	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	waitClosed(t, m)
	close(release)
	recv(t, closed)

	if got := mustGet(t, m, failed.ID); got.Status != StatusFailed || got.Error != "permission denied" {
		t.Fatalf("a removal that fails during Close is failed, not cancelled: %+v", got)
	}
	if got := mustGet(t, m, completed.ID); got.Status != StatusCompleted {
		t.Fatalf("a removal that finishes during Close is completed: %+v", got)
	}
}

func TestFollowLogsEndsWhenPendingJobIsCancelledOrRunnerCloses(t *testing.T) {
	m := NewMemory(Options{MaxRunning: 1})
	g := newGate()
	running := mustSubmit(t, m, uuid.New(), TypeInstall, g.body)
	g.waitStarted(t)
	cancelled := mustSubmit(t, m, uuid.New(), TypeInstall, noop)
	closed := mustSubmit(t, m, uuid.New(), TypeInstall, noop)

	follow := func(id uuid.UUID) chan error {
		done := make(chan error, 1)
		go func() { done <- m.FollowLogs(context.Background(), id, io.Discard) }()
		return done
	}
	cancelledDone, closedDone, runningDone := follow(cancelled.ID), follow(closed.ID), follow(running.ID)

	if err := m.Cancel(context.Background(), cancelled.ID); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, cancelledDone); err != nil {
		t.Fatalf("follower of a job cancelled while pending: %v", err)
	}
	m.Close()
	for _, done := range []chan error{closedDone, runningDone} {
		if err := recv(t, done); err != nil {
			t.Fatalf("follower after Close: %v", err)
		}
	}
}

// --- Logs ---------------------------------------------------------------

func TestLogsKeepOutput(t *testing.T) {
	m := newRunner(t, Options{})
	var leaked io.Writer
	j := mustSubmit(t, m, uuid.New(), TypeCreate, func(_ context.Context, log io.Writer) error {
		leaked = log
		io.WriteString(log, "solving\n")
		fmt.Fprintf(log, "done in %ds\n", 3)
		return nil
	})
	mustWait(t, m, j.ID)
	if got, err := m.Logs(j.ID); err != nil || string(got) != "solving\ndone in 3s\n" {
		t.Fatalf("Logs = %q, %v", got, err)
	}
	if n, err := leaked.Write([]byte("late\n")); n != 5 || err != nil {
		t.Fatalf("a write after the job finished must not fail: %d, %v", n, err)
	}
	if got, _ := m.Logs(j.ID); string(got) != "solving\ndone in 3s\n" {
		t.Fatalf("output after the job finished must be discarded, got %q", got)
	}
}

func TestLogsAreSafeForConcurrentWriters(t *testing.T) {
	m := newRunner(t, Options{})
	const lines = 500
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(_ context.Context, log io.Writer) error {
		var wg sync.WaitGroup
		for _, stream := range []string{"out", "err"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range lines {
					fmt.Fprintf(log, "%s %03d\n", stream, i)
				}
			}()
		}
		wg.Wait()
		return nil
	})
	mustWait(t, m, j.ID)
	got, _ := m.Logs(j.ID)
	if n := strings.Count(string(got), "\n"); n != 2*lines {
		t.Fatalf("got %d lines, want %d", n, 2*lines)
	}
}

func TestLogsAreBounded(t *testing.T) {
	m := newRunner(t, Options{LogLimit: 16})
	var all bytes.Buffer
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(_ context.Context, log io.Writer) error {
		for i := range 40 {
			line := fmt.Sprintf("line %d\n", i)
			all.WriteString(line)
			io.WriteString(log, line)
		}
		return nil
	})
	mustWait(t, m, j.ID)
	got, _ := m.Logs(j.ID)
	if want := all.Bytes()[all.Len()-16:]; !bytes.Equal(got, want) {
		t.Fatalf("Logs = %q, want the last 16 bytes %q", got, want)
	}
}

func TestLogsDefaultLimitIsOneMiB(t *testing.T) {
	m := newRunner(t, Options{})
	const total = 3 << 20
	chunk := make([]byte, 64<<10)
	all := make([]byte, 0, total)
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(_ context.Context, log io.Writer) error {
		for i := 0; len(all) < total; i++ {
			for k := range chunk {
				chunk[k] = byte(i + k)
			}
			all = append(all, chunk...)
			log.Write(chunk)
		}
		return nil
	})
	mustWait(t, m, j.ID)
	got, _ := m.Logs(j.ID)
	if len(got) != 1<<20 || !bytes.Equal(got, all[len(all)-1<<20:]) {
		t.Fatalf("kept %d bytes, want the last 1 MiB", len(got))
	}
}

func TestTailBufferKeepsLastBytes(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, limit := range []int{1, 7, 64} {
		b := tailBuffer{max: limit}
		var all []byte
		for range 2000 {
			p := make([]byte, rng.Intn(3*limit+1))
			rng.Read(p)
			all = append(all, p...)
			b.Write(p)
			want := all[max(0, len(all)-limit):]
			if got := b.bytesFrom(0); !bytes.Equal(got, want) {
				t.Fatalf("limit %d after %d bytes: got %x, want %x", limit, len(all), got, want)
			}
			for i := 0; i <= len(want); i++ {
				if got := b.bytesFrom(i); !bytes.Equal(got, want[i:]) {
					t.Fatalf("limit %d after %d bytes, from %d: got %x, want %x", limit, len(all), i, got, want[i:])
				}
			}
			if len(b.buf) > limit {
				t.Fatalf("limit %d: buffer holds %d bytes", limit, len(b.buf))
			}
		}
	}
}

// syncBuffer is a writer the test can read while FollowLogs writes to it.
type syncBuffer struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	changed chan struct{}
}

func newSyncBuffer() *syncBuffer { return &syncBuffer{changed: make(chan struct{}, 1)} }

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case b.changed <- struct{}{}:
	default:
	}
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitFor waits until the buffer holds exactly want.
func (b *syncBuffer) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.After(timeout)
	for b.String() != want {
		select {
		case <-b.changed:
		case <-deadline:
			t.Fatalf("follower has %q, want %q", b.String(), want)
		}
	}
}

func TestFollowLogs(t *testing.T) {
	m := newRunner(t, Options{MaxRunning: 1})
	blocker := newGate()
	mustSubmit(t, m, uuid.New(), TypeInstall, blocker.body)
	blocker.waitStarted(t)

	step1, step2 := make(chan struct{}), make(chan struct{})
	wrote1 := make(chan struct{})
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(ctx context.Context, log io.Writer) error {
		io.WriteString(log, "one\n")
		close(wrote1)
		for _, step := range []chan struct{}{step1, step2} {
			select {
			case <-step:
			case <-ctx.Done():
				return ctx.Err()
			}
			if step == step1 {
				io.WriteString(log, "two\n")
			}
		}
		io.WriteString(log, "three\n")
		return nil
	})

	type follower struct {
		buf  *syncBuffer
		done chan error
	}
	follow := func() follower {
		f := follower{buf: newSyncBuffer(), done: make(chan error, 1)}
		go func() { f.done <- m.FollowLogs(context.Background(), j.ID, f.buf) }()
		return f
	}

	early := follow() // follows from before the job starts
	blocker.open()
	recv(t, wrote1)
	mid := follow() // gets "one\n" from the buffer, then follows
	early.buf.waitFor(t, "one\n")
	mid.buf.waitFor(t, "one\n")

	close(step1)
	// New output reaches the followers while the job is still running.
	early.buf.waitFor(t, "one\ntwo\n")
	mid.buf.waitFor(t, "one\ntwo\n")
	wantStatus(t, m, j.ID, StatusRunning)
	select {
	case err := <-early.done:
		t.Fatalf("FollowLogs returned before the job finished: %v", err)
	default:
	}

	close(step2)
	for _, f := range []follower{early, mid} {
		if err := recv(t, f.done); err != nil {
			t.Fatalf("FollowLogs: %v", err)
		}
		if got := f.buf.String(); got != "one\ntwo\nthree\n" {
			t.Fatalf("follower got %q", got)
		}
	}

	// A reader that comes after the job finished gets the kept output.
	late := newSyncBuffer()
	if err := m.FollowLogs(waitCtx(t), j.ID, late); err != nil || late.String() != "one\ntwo\nthree\n" {
		t.Fatalf("late follower: %q, %v", late.String(), err)
	}
}

// blockingWriter blocks its first Write until release is closed, standing
// in for a reader that cannot keep up.
type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	buf     bytes.Buffer
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.entered)
		<-w.release
	})
	return w.buf.Write(p)
}

func TestSlowFollowerSkipsDroppedOutputWithoutBlockingJob(t *testing.T) {
	const limit = 10
	m := newRunner(t, Options{LogLimit: limit})
	wrote, more := make(chan struct{}), make(chan struct{})
	burst := strings.Repeat("0123456789", 3)
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(ctx context.Context, log io.Writer) error {
		io.WriteString(log, "abc")
		close(wrote)
		select {
		case <-more:
		case <-ctx.Done():
			return ctx.Err()
		}
		io.WriteString(log, burst)
		return nil
	})
	recv(t, wrote)

	w := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- m.FollowLogs(context.Background(), j.ID, w) }()
	recv(t, w.entered) // the follower is stuck writing "abc"

	// The job writes three times its log limit and finishes while the
	// follower is stuck: a slow reader must not hold it back.
	close(more)
	if got := mustWait(t, m, j.ID); got.Status != StatusCompleted {
		t.Fatalf("job: %+v", got)
	}

	close(w.release)
	if err := recv(t, done); err != nil {
		t.Fatalf("FollowLogs: %v", err)
	}
	skipped := len(burst) - limit
	want := "abc" + fmt.Sprintf("\n[nebi] %d bytes of output skipped: this reader fell behind, and a job keeps only its last %d bytes\n", skipped, limit) + burst[len(burst)-limit:]
	if got := w.buf.String(); got != want {
		t.Fatalf("slow follower got %q, want %q", got, want)
	}
}

func TestFollowLogsStopsWithContextAndWriterErrors(t *testing.T) {
	m := newRunner(t, Options{})
	g := newGate()
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(ctx context.Context, log io.Writer) error {
		io.WriteString(log, "hello\n")
		return g.body(ctx, log)
	})
	g.waitStarted(t)

	ctx, cancel := context.WithCancel(context.Background())
	buf := newSyncBuffer()
	done := make(chan error, 1)
	go func() { done <- m.FollowLogs(ctx, j.ID, buf) }()
	buf.waitFor(t, "hello\n")
	cancel()
	if err := recv(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("FollowLogs after ctx is cancelled: want context.Canceled, got %v", err)
	}

	errWrite := errors.New("client went away")
	if err := m.FollowLogs(waitCtx(t), j.ID, failingWriter{errWrite}); !errors.Is(err, errWrite) {
		t.Fatalf("FollowLogs with a failing writer: want its error, got %v", err)
	}
	g.open()
	mustWait(t, m, j.ID)
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// --- helpers reading the runner's state ----------------------------------

func projectJobs(t *testing.T, m *Memory, projectID uuid.UUID) []Job {
	t.Helper()
	list, err := m.ProjectJobs(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func ids(list []Job) []uuid.UUID {
	out := make([]uuid.UUID, len(list))
	for i, j := range list {
		out[i] = j.ID
	}
	return out
}

func equalIDs(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func jobCount(m *Memory) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.jobs)
}

func retiredCount(m *Memory) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.retired)
}
