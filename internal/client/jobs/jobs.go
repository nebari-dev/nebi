// Package jobs runs the client's background jobs: long operations such as
// solving a project's lock file or removing its directory, which callers
// start and then follow by job ID instead of waiting on them.
//
// Memory is an in-process runner. Jobs do not survive a restart of the
// process that ran them, and one runner does not see the jobs of another,
// so every client process that shares a data directory must go through the
// same runner for its busy checks to hold.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Type says what a job does.
type Type string

const (
	// TypeCreate solves a new project's lock file (and installs it when
	// requested) and records version 1.
	TypeCreate Type = "create"
	// TypeInstall materializes a project's environment.
	TypeInstall Type = "install"
	// TypeUninstall removes a project's environment.
	TypeUninstall Type = "uninstall"
	// TypeDeleteFiles removes a deleted project's directory from disk.
	TypeDeleteFiles Type = "delete_files"
)

// Cancellable reports whether jobs of this type can be cancelled. A file
// removal job cannot: stopping it part way would leave a half-removed
// directory.
func (t Type) Cancellable() bool {
	return t != TypeDeleteFiles
}

// Status is where a job is in its life cycle. The values match the job
// statuses of the server (internal/models.JobStatus) and the frontend.
type Status string

const (
	// StatusPending: the job waits for a free slot, or for the job running
	// for its project to finish.
	StatusPending Status = "pending"
	// StatusRunning: the job's body is running.
	StatusRunning Status = "running"
	// StatusCompleted: the body returned nil.
	StatusCompleted Status = "completed"
	// StatusFailed: the body returned an error or panicked.
	StatusFailed Status = "failed"
	// StatusCancelled: the job was cancelled, or the runner was closed,
	// before it started, or while it ran and its body returned an error.
	StatusCancelled Status = "cancelled"
)

// Defaults for the zero values of Options.
const (
	DefaultMaxRunning   = 2
	DefaultKeepFinished = 200
	DefaultLogLimit     = 1 << 20 // 1 MiB
)

// Options configures a Memory runner. Zero values take the defaults.
type Options struct {
	// MaxRunning is the most job bodies that run at the same time, across
	// every project and job type. Values below 1 mean DefaultMaxRunning.
	MaxRunning int
	// KeepFinished is how many finished jobs are kept, beyond each
	// project's latest finished job: the most recently finished across all
	// projects. Values below 1 mean DefaultKeepFinished.
	KeepFinished int
	// LogLimit is how many bytes of output are kept per job; older output
	// is dropped. Values below 1 mean DefaultLogLimit.
	LogLimit int
}

// Job is a snapshot of a background job.
type Job struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Type      Type      `json:"type"`
	Status    Status    `json:"status"`
	// Stage is the last step the job reported with ReportStage, e.g. "lock"
	// or "install". Empty until the job reports one.
	Stage string `json:"stage,omitempty"`
	// Error is why a failed job failed. Empty for other statuses.
	Error string `json:"error,omitempty"`
	// VersionID is the version the job recorded, reported with
	// ReportVersion. Versions do not point at jobs; the job carries it.
	VersionID *uuid.UUID `json:"version_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	// StartedAt is nil until the body starts, and stays nil for a job
	// cancelled while pending.
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Active reports whether the job is pending or running.
func (j Job) Active() bool {
	return j.Status == StatusPending || j.Status == StatusRunning
}

// Func is the body of a job. Output written to log is kept with the job
// and can be followed with FollowLogs; pixi's stdout and stderr go there.
// log is safe for concurrent use.
//
// The context is cancelled when the job is cancelled or the runner is
// closed. The body must then return promptly, so every process it starts
// must be started with ctx (for pixi, through process.CommandContext,
// which also stops pixi's child processes).
type Func func(ctx context.Context, log io.Writer) error

// ErrClosed is returned by Submit after Close.
var ErrClosed = errors.New("job runner is closed")

// ErrProjectRetired is returned by Submit for a project retired with
// RetireProject, for job types it did not allow.
var ErrProjectRetired = errors.New("project no longer accepts jobs")

// ErrUnknownJob is returned for a job ID the runner does not know: one it
// never ran, or a finished job it has since dropped.
var ErrUnknownJob = errors.New("unknown job")

// ErrNotCancellable is matched by the *NotCancellableError Cancel returns
// for an active job whose type cannot be cancelled.
var ErrNotCancellable = errors.New("job cannot be cancelled")

// NotCancellableError is returned by Cancel for a pending or running job
// of a type that cannot be cancelled (see Type.Cancellable). It matches
// ErrNotCancellable with errors.Is.
type NotCancellableError struct {
	JobID uuid.UUID
	Type  Type
}

func (e *NotCancellableError) Error() string {
	return fmt.Sprintf("%v: %s job %s", ErrNotCancellable, e.Type, e.JobID)
}

// Unwrap makes errors.Is(err, ErrNotCancellable) true.
func (e *NotCancellableError) Unwrap() error { return ErrNotCancellable }

type ctxKey struct{}

type jobCtx struct {
	id            uuid.UUID
	report        func(stage string)
	reportVersion func(id uuid.UUID)
}

// IDFromContext returns the ID of the job whose body is running with ctx.
func IDFromContext(ctx context.Context) (uuid.UUID, bool) {
	jc, ok := ctx.Value(ctxKey{}).(*jobCtx)
	if !ok {
		return uuid.Nil, false
	}
	return jc.id, true
}

// ReportStage records the step the running job has reached. It does nothing
// when ctx does not belong to a job.
func ReportStage(ctx context.Context, stage string) {
	if jc, ok := ctx.Value(ctxKey{}).(*jobCtx); ok {
		jc.report(stage)
	}
}

// ReportVersion records the version the running job created. It does
// nothing when ctx does not belong to a job.
func ReportVersion(ctx context.Context, versionID uuid.UUID) {
	if jc, ok := ctx.Value(ctxKey{}).(*jobCtx); ok {
		jc.reportVersion(versionID)
	}
}

// Memory runs submitted jobs in goroutines and keeps their state in memory.
//
// At most Options.MaxRunning jobs run at once, and at most one per project.
// The others wait as pending and start in submission order, skipping a job
// whose project is busy so it does not hold back jobs for other projects.
//
// Finished jobs are dropped when they are no longer needed: each project's
// latest finished job is kept while the project exists, and the others are
// kept up to Options.KeepFinished, most recently finished first. Active
// jobs are never dropped. Jobs are dropped as other jobs finish.
type Memory struct {
	maxRunning   int
	keepFinished int
	logLimit     int

	mu     sync.Mutex
	jobs   map[uuid.UUID]*entry
	byProj map[uuid.UUID][]*entry // each project's jobs in submission order
	// pending holds the jobs waiting to start, in submission order.
	pending []*entry
	running int
	// busy holds the projects that have a running job.
	busy map[uuid.UUID]bool
	// finished holds the kept finished jobs, in the order they finished.
	finished []*entry
	// retired maps a retired project to the job types it still accepts.
	retired map[uuid.UUID]map[Type]bool
	closed  bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type entry struct {
	job  Job
	run  Func // nil once the job has started or ended
	done chan struct{}
	log  *jobLog
	// cancel cancels the body's context; set while the job runs.
	cancel context.CancelFunc
	// cancelled is set when Cancel or Close asks a running job to stop.
	cancelled bool
}

// NewMemory returns a ready runner. Call Close when done with it.
func NewMemory(opts Options) *Memory {
	if opts.MaxRunning < 1 {
		opts.MaxRunning = DefaultMaxRunning
	}
	if opts.KeepFinished < 1 {
		opts.KeepFinished = DefaultKeepFinished
	}
	if opts.LogLimit < 1 {
		opts.LogLimit = DefaultLogLimit
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Memory{
		maxRunning:   opts.MaxRunning,
		keepFinished: opts.KeepFinished,
		logLimit:     opts.LogLimit,
		jobs:         make(map[uuid.UUID]*entry),
		byProj:       make(map[uuid.UUID][]*entry),
		busy:         make(map[uuid.UUID]bool),
		retired:      make(map[uuid.UUID]map[Type]bool),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Submit adds run as a job of the given type for projectID and returns a
// snapshot of it: running when it could start at once, pending otherwise.
// The job outlives ctx; only Cancel and Close stop it.
func (m *Memory) Submit(_ context.Context, projectID uuid.UUID, typ Type, run Func) (*Job, error) {
	if run == nil {
		return nil, errors.New("job body is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if allowed, ok := m.retired[projectID]; ok && !allowed[typ] {
		return nil, fmt.Errorf("%w: %s job for project %s", ErrProjectRetired, typ, projectID)
	}
	id := uuid.New()
	e := &entry{
		job: Job{
			ID:        id,
			ProjectID: projectID,
			Type:      typ,
			Status:    StatusPending,
			CreatedAt: time.Now(),
		},
		run:  run,
		done: make(chan struct{}),
		log:  newJobLog(m.logLimit),
	}
	m.jobs[id] = e
	m.byProj[projectID] = append(m.byProj[projectID], e)
	m.pending = append(m.pending, e)
	m.schedule()
	job := e.job
	return &job, nil
}

// schedule starts pending jobs, oldest first, while there is a free slot,
// skipping jobs whose project already has one running. Called with mu held.
func (m *Memory) schedule() {
	if m.closed {
		return
	}
	kept := m.pending[:0]
	for _, e := range m.pending {
		if m.running < m.maxRunning && !m.busy[e.job.ProjectID] {
			m.start(e)
			continue
		}
		kept = append(kept, e)
	}
	clear(m.pending[len(kept):])
	m.pending = kept
}

// start marks e running and runs its body in a goroutine. Called with mu
// held.
func (m *Memory) start(e *entry) {
	m.running++
	m.busy[e.job.ProjectID] = true
	started := time.Now()
	e.job.Status = StatusRunning
	e.job.StartedAt = &started

	ctx, cancel := context.WithCancel(m.ctx)
	e.cancel = cancel
	ctx = context.WithValue(ctx, ctxKey{}, &jobCtx{
		id: e.job.ID,
		report: func(stage string) {
			m.mu.Lock()
			e.job.Stage = stage
			m.mu.Unlock()
		},
		reportVersion: func(versionID uuid.UUID) {
			m.mu.Lock()
			e.job.VersionID = &versionID
			m.mu.Unlock()
		},
	})
	run := e.run
	e.run = nil
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		err := call(ctx, run, e.log)
		// Only a cancel that reached the body before it returned makes the
		// job cancelled; a body that had already failed on its own stays
		// failed.
		interrupted := ctx.Err() != nil
		cancel()
		m.mu.Lock()
		defer m.mu.Unlock()
		m.running--
		delete(m.busy, e.job.ProjectID)
		switch {
		case err == nil:
			m.finish(e, StatusCompleted, "")
		case e.cancelled && interrupted:
			m.finish(e, StatusCancelled, "")
		default:
			m.finish(e, StatusFailed, err.Error())
		}
		m.schedule()
	}()
}

func call(ctx context.Context, run Func, log io.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("job panicked: %v", r)
		}
	}()
	return run(ctx, log)
}

// finish records the end of a job, ends its log, wakes its waiters and
// drops the finished jobs no longer kept. Called with mu held.
func (m *Memory) finish(e *entry, status Status, errMsg string) {
	finished := time.Now()
	e.job.Status = status
	e.job.Error = errMsg
	e.job.FinishedAt = &finished
	e.run = nil
	e.cancel = nil
	e.log.finish()
	close(e.done)
	m.finished = append(m.finished, e)
	if e.job.Type == TypeDeleteFiles && status == StatusCompleted {
		// The project's files are gone, so the project is gone for good.
		delete(m.retired, e.job.ProjectID)
	}
	m.evict()
}

// evict drops finished jobs beyond the kept ones: each existing project's
// latest finished job, and the KeepFinished most recently finished others.
// A project counts as gone while it is retired, and once its latest job is
// a file removal, which only runs for a deleted project. Called with mu
// held.
func (m *Memory) evict() {
	pinned := func(e *entry) bool {
		if _, retired := m.retired[e.job.ProjectID]; retired || e.job.Type == TypeDeleteFiles {
			return false
		}
		list := m.byProj[e.job.ProjectID]
		for i := len(list) - 1; i >= 0; i-- {
			if !list[i].job.Active() {
				return list[i] == e
			}
		}
		return false
	}
	excess := -m.keepFinished
	for _, e := range m.finished {
		if !pinned(e) {
			excess++
		}
	}
	if excess <= 0 {
		return
	}
	kept := m.finished[:0]
	for _, e := range m.finished {
		if excess > 0 && !pinned(e) {
			m.drop(e)
			excess--
			continue
		}
		kept = append(kept, e)
	}
	clear(m.finished[len(kept):])
	m.finished = kept
}

// drop forgets a finished job. Called with mu held.
func (m *Memory) drop(e *entry) {
	delete(m.jobs, e.job.ID)
	pid := e.job.ProjectID
	list := slices.DeleteFunc(m.byProj[pid], func(o *entry) bool { return o == e })
	if len(list) == 0 {
		delete(m.byProj, pid)
	} else {
		m.byProj[pid] = list
	}
}

// Cancel cancels a job. A pending job ends as cancelled without running. A
// running job has its context cancelled and ends as cancelled when its body
// returns an error; Cancel does not wait for that. Cancelling a finished
// job does nothing. Cancel returns a *NotCancellableError for an active job
// whose type cannot be cancelled, and ErrUnknownJob for an unknown ID.
func (m *Memory) Cancel(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownJob, id)
	}
	return m.cancelLocked(e)
}

// cancelLocked is Cancel for a known job. Called with mu held.
func (m *Memory) cancelLocked(e *entry) error {
	if !e.job.Active() {
		return nil
	}
	if !e.job.Type.Cancellable() {
		return &NotCancellableError{JobID: e.job.ID, Type: e.job.Type}
	}
	if e.job.Status == StatusPending {
		m.pending = slices.DeleteFunc(m.pending, func(o *entry) bool { return o == e })
		m.finish(e, StatusCancelled, "")
		return nil
	}
	e.cancelled = true
	e.cancel()
	return nil
}

// RetireProject atomically checks that the project has no pending or
// running job and, when it has none, makes Submit refuse every later job
// for it except those of the allowed types. When jobs are active it
// returns their IDs and retires nothing. Deleting a project retires it, so
// no job can start between the busy check and the delete.
//
// A retired project counts as gone: its latest finished job is no longer
// kept for it. The retirement ends when a file removal job of the project
// completes, or with RestoreProject.
func (m *Memory) RetireProject(_ context.Context, projectID uuid.UUID, allow ...Type) ([]uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var active []uuid.UUID
	for _, e := range m.byProj[projectID] {
		if e.job.Active() {
			active = append(active, e.job.ID)
		}
	}
	if len(active) > 0 {
		return active, nil
	}
	allowed := make(map[Type]bool, len(allow))
	for _, t := range allow {
		allowed[t] = true
	}
	m.retired[projectID] = allowed
	return nil, nil
}

// RestoreProject undoes RetireProject, for a delete that did not happen.
func (m *Memory) RestoreProject(_ context.Context, projectID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.retired, projectID)
	return nil
}

// Get returns a snapshot of the job.
func (m *Memory) Get(id uuid.UUID) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("%w: %s", ErrUnknownJob, id)
	}
	return e.job, nil
}

// ProjectJobs returns snapshots of the project's jobs, oldest first.
func (m *Memory) ProjectJobs(_ context.Context, projectID uuid.UUID) ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.byProj[projectID]
	if len(list) == 0 {
		return nil, nil
	}
	out := make([]Job, len(list))
	for i, e := range list {
		out[i] = e.job
	}
	return out, nil
}

// Wait blocks until the job has finished or ctx is done, and returns the
// job's final snapshot, even when the job has been dropped since.
func (m *Memory) Wait(ctx context.Context, id uuid.UUID) (Job, error) {
	m.mu.Lock()
	e, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return Job{}, fmt.Errorf("%w: %s", ErrUnknownJob, id)
	}
	select {
	case <-e.done:
		m.mu.Lock()
		defer m.mu.Unlock()
		return e.job, nil
	case <-ctx.Done():
		return Job{}, ctx.Err()
	}
}

// Logs returns the job's kept output: the last Options.LogLimit bytes it
// wrote.
func (m *Memory) Logs(id uuid.UUID) ([]byte, error) {
	m.mu.Lock()
	e, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownJob, id)
	}
	b, _, _, _, _ := e.log.readFrom(-1)
	return b, nil
}

// FollowLogs writes the job's kept output to w, then its new output as the
// job writes it, and returns nil once the job has finished and all of it is
// written. It returns early with ctx's error when ctx is done, or with w's
// error when a write fails. Several readers can follow one job.
//
// Each reader reads the job's kept output at its own pace, so a slow reader
// never blocks the job and holds no output of its own: memory stays bounded
// by Options.LogLimit per job. A reader that falls more than LogLimit bytes
// behind skips the output no longer kept, and a marker line in the stream
// says how many bytes it skipped.
func (m *Memory) FollowLogs(ctx context.Context, id uuid.UUID, w io.Writer) error {
	m.mu.Lock()
	e, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownJob, id)
	}
	off := int64(-1) // start at the oldest kept byte
	for {
		data, skipped, next, done, changed := e.log.readFrom(off)
		if skipped > 0 {
			if _, err := fmt.Fprintf(w, "\n[nebi] %d bytes of output skipped: this reader fell behind, and a job keeps only its last %d bytes\n", skipped, m.logLimit); err != nil {
				return err
			}
		}
		if len(data) > 0 {
			if _, err := w.Write(data); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
		off = next
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Close cancels pending and running jobs, waits for running ones to
// return and rejects new submissions. The jobs it stops end as cancelled.
func (m *Memory) Close() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		pending := m.pending
		m.pending = nil
		for _, e := range pending {
			m.finish(e, StatusCancelled, "")
		}
		for _, e := range m.jobs {
			if e.job.Status == StatusRunning {
				e.cancelled = true
			}
		}
	}
	m.mu.Unlock()
	m.cancel()
	m.wg.Wait()
}

// jobLog is a job's output: the last bytes it wrote, which Logs returns
// and followers read from at their own pace.
type jobLog struct {
	mu  sync.Mutex
	buf tailBuffer
	// total counts every byte written; the kept bytes are the last
	// len(buf.buf) of them.
	total int64
	done  bool
	// changed is closed when output is written or the log ends, to wake
	// the followers. A write replaces it with a new one.
	changed chan struct{}
}

func newJobLog(limit int) *jobLog {
	return &jobLog{buf: tailBuffer{max: limit}, changed: make(chan struct{})}
}

// Write keeps p and wakes the followers. Output written after the job
// finished is discarded.
func (l *jobLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done || len(p) == 0 {
		return len(p), nil
	}
	l.buf.Write(p)
	l.total += int64(len(p))
	close(l.changed)
	l.changed = make(chan struct{})
	return len(p), nil
}

// readFrom returns a copy of the output from byte offset off on, the
// offset after it, whether the log has ended, and a channel closed on the
// next change. skipped counts the bytes from off on that are no longer
// kept. A negative off reads from the oldest kept byte.
func (l *jobLog) readFrom(off int64) (data []byte, skipped, next int64, done bool, changed <-chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	oldest := l.total - int64(len(l.buf.buf))
	if off < oldest {
		if off >= 0 {
			skipped = oldest - off
		}
		off = oldest
	}
	return l.buf.bytesFrom(int(off - oldest)), skipped, l.total, l.done, l.changed
}

// finish ends the log: later writes are discarded and followers return.
func (l *jobLog) finish() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.done {
		l.done = true
		close(l.changed)
	}
}

// tailBuffer keeps the last max bytes written to it. It grows as output
// arrives, up to max, and then wraps around.
type tailBuffer struct {
	max   int
	buf   []byte // len(buf) <= max
	start int    // index of the oldest byte once len(buf) == max; else 0
}

func (b *tailBuffer) Write(p []byte) {
	if len(p) >= b.max {
		b.buf = append(b.buf[:0], p[len(p)-b.max:]...)
		b.start = 0
		return
	}
	if room := b.max - len(b.buf); room > 0 {
		n := min(room, len(p))
		b.buf = append(b.buf, p[:n]...)
		p = p[n:]
	}
	for len(p) > 0 {
		n := copy(b.buf[b.start:], p)
		p = p[n:]
		b.start = (b.start + n) % b.max
	}
}

// Bytes returns a copy of the kept bytes, oldest first.
func (b *tailBuffer) Bytes() []byte {
	return b.bytesFrom(0)
}

// bytesFrom returns a copy of the kept bytes from the i-th oldest on.
func (b *tailBuffer) bytesFrom(i int) []byte {
	out := make([]byte, max(len(b.buf)-i, 0))
	if len(out) == 0 {
		return out
	}
	if p := b.start + i; p < len(b.buf) {
		n := copy(out, b.buf[p:])
		copy(out[n:], b.buf[:b.start])
	} else {
		copy(out, b.buf[p-len(b.buf):b.start])
	}
	return out
}
