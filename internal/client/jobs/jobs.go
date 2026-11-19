// Package jobs runs the client's background jobs: long operations such as
// solving a project's lock file or removing its directory, which callers
// start and then follow by job ID instead of waiting on them.
//
// Memory is an in-process runner. Jobs do not survive a restart of the
// process that ran them, and one runner does not see the jobs of another,
// so every client process that shares a data directory must go through the
// same runner for its busy checks to hold. The owner lock (TryAcquireOwner,
// AcquireOwner, ReadOwner) decides which process runs it; see Owner.
package jobs

import (
	"context"
	"errors"
	"fmt"
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

// Status is where a job is in its life cycle.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// Job is a snapshot of a background job.
type Job struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Type      Type      `json:"type"`
	Status    Status    `json:"status"`
	// Stage is the last step the job reported with ReportStage, e.g. "lock"
	// or "install". Empty until the job reports one.
	Stage string `json:"stage,omitempty"`
	Error string `json:"error,omitempty"`
	// VersionID is the version the job recorded, reported with
	// ReportVersion. Versions do not point at jobs; the job carries it.
	VersionID  *uuid.UUID `json:"version_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Active reports whether the job is queued or running.
func (j Job) Active() bool {
	return j.Status == StatusQueued || j.Status == StatusRunning
}

// Func is the body of a job. The context is cancelled when the runner is
// closed.
type Func func(ctx context.Context) error

// ErrClosed is returned by Submit after Close.
var ErrClosed = errors.New("job runner is closed")

// ErrProjectRetired is returned by Submit for a project retired with
// RetireProject, for job types it did not allow.
var ErrProjectRetired = errors.New("project no longer accepts jobs")

// ErrUnknownJob is returned for a job ID the runner does not know.
var ErrUnknownJob = errors.New("unknown job")

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

// Memory runs each submitted job in its own goroutine and keeps every job's
// state in memory.
type Memory struct {
	mu     sync.Mutex
	jobs   map[uuid.UUID]*entry
	order  []uuid.UUID // submission order
	closed bool
	// retired maps a retired project to the job types it still accepts.
	retired map[uuid.UUID]map[Type]bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type entry struct {
	job  Job
	done chan struct{}
}

// NewMemory returns a ready runner. Call Close when done with it.
func NewMemory() *Memory {
	ctx, cancel := context.WithCancel(context.Background())
	return &Memory{
		jobs:    make(map[uuid.UUID]*entry),
		retired: make(map[uuid.UUID]map[Type]bool),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Submit starts run as a job of the given type for projectID and returns
// the queued job. The job outlives ctx; only Close cancels it.
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
	e := &entry{
		job: Job{
			ID:        uuid.New(),
			ProjectID: projectID,
			Type:      typ,
			Status:    StatusQueued,
			CreatedAt: time.Now(),
		},
		done: make(chan struct{}),
	}
	m.jobs[e.job.ID] = e
	m.order = append(m.order, e.job.ID)
	m.wg.Add(1)
	go m.run(e, run)
	job := e.job
	return &job, nil
}

func (m *Memory) run(e *entry, run Func) {
	defer m.wg.Done()
	defer close(e.done)

	m.mu.Lock()
	started := time.Now()
	e.job.Status = StatusRunning
	e.job.StartedAt = &started
	id := e.job.ID
	m.mu.Unlock()

	ctx := context.WithValue(m.ctx, ctxKey{}, &jobCtx{
		id: id,
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
	err := call(ctx, run)

	m.mu.Lock()
	finished := time.Now()
	e.job.FinishedAt = &finished
	if err != nil {
		e.job.Status = StatusFailed
		e.job.Error = err.Error()
	} else {
		e.job.Status = StatusSucceeded
	}
	m.mu.Unlock()
}

func call(ctx context.Context, run Func) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("job panicked: %v", r)
		}
	}()
	return run(ctx)
}

// RetireProject atomically checks that the project has no queued or
// running job and, when it has none, makes Submit refuse every later job
// for it except those of the allowed types. When jobs are active it
// returns their IDs and retires nothing. Deleting a project retires it, so
// no job can start between the busy check and the delete.
func (m *Memory) RetireProject(_ context.Context, projectID uuid.UUID, allow ...Type) ([]uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var active []uuid.UUID
	for _, id := range m.order {
		if e := m.jobs[id]; e.job.ProjectID == projectID && e.job.Active() {
			active = append(active, id)
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
	var out []Job
	for _, id := range m.order {
		if e := m.jobs[id]; e.job.ProjectID == projectID {
			out = append(out, e.job)
		}
	}
	return out, nil
}

// Wait blocks until the job has finished or ctx is done, and returns the
// job's final snapshot.
func (m *Memory) Wait(ctx context.Context, id uuid.UUID) (Job, error) {
	m.mu.Lock()
	e, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return Job{}, fmt.Errorf("%w: %s", ErrUnknownJob, id)
	}
	select {
	case <-e.done:
		return m.Get(id)
	case <-ctx.Done():
		return Job{}, ctx.Err()
	}
}

// Close cancels running jobs, waits for them to return and rejects new
// submissions.
func (m *Memory) Close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	m.wg.Wait()
}
