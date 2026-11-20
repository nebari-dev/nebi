// Package local is the client SDK for projects on this machine: the local
// Project data model and its create, list, get, rename and delete
// operations (issue #660), and the immutable versions and movable tags of a
// project with their create, read and tag operations (issue #661).
// Everything here runs on the user's machine; the server is never
// contacted, and a project exists on a server only after it is pushed.
//
// The SDK owns its tables in the client database (nebi.db) and shares none
// with the server models in internal/models.
//
// Work that belongs to other parts of the client is reached through small
// interfaces, so each part can be built and tested on its own:
//   - JobRunner: background jobs (internal/client/jobs)
//   - PixiRunner: pixi lock, install and lock check (internal/client/sdk/exec)
package local

import (
	"context"
	"errors"
	"io"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
)

// JobRunner submits and reports background jobs (internal/client/jobs).
// *jobs.Memory implements it.
type JobRunner interface {
	// Submit adds run as a job of the given type for the project. The job
	// is running when it could start at once, and pending while it waits
	// for a free slot or for the project's running job to finish.
	Submit(ctx context.Context, projectID uuid.UUID, typ jobs.Type, run jobs.Func) (*jobs.Job, error)
	// ProjectJobs returns the project's jobs the runner still keeps,
	// oldest first: every pending or running job, the latest finished one
	// of each type that started while the project exists, and older
	// finished ones only until they are dropped.
	//
	// Project reads rely on when this list changes (see readProjects): a
	// job is listed before its body starts, so before it writes anything,
	// and is marked finished, meaning no longer active, only after its
	// body has returned, so after every write it makes. A runner must keep
	// both: a job that writes before it is listed, or is marked finished
	// before its writes are done, can make a read report a state the
	// project was never in. The list may change at other times too, such
	// as when an old finished job is dropped; that only costs a read a
	// retry.
	ProjectJobs(ctx context.Context, projectID uuid.UUID) ([]jobs.Job, error)
	// RetireProject atomically checks that the project has no pending or
	// running job and, when it has none, refuses every later job for it
	// except the allowed types. When jobs are active it returns their IDs
	// and changes nothing.
	RetireProject(ctx context.Context, projectID uuid.UUID, allow ...jobs.Type) ([]uuid.UUID, error)
	// RestoreProject undoes RetireProject.
	RestoreProject(ctx context.Context, projectID uuid.UUID) error
	// ProjectDeleted reports that the project's delete has committed, after
	// any job the delete still runs for it was submitted. It ends the
	// retirement, and the runner no longer keeps the project's finished
	// jobs for it.
	ProjectDeleted(ctx context.Context, projectID uuid.UUID) error
	// Cancel cancels a job: a pending job ends as cancelled without
	// running, and a running job has its context cancelled. Cancelling a
	// finished job does nothing. A file removal job cannot be cancelled;
	// for one that is active, Cancel returns a *jobs.NotCancellableError.
	Cancel(ctx context.Context, jobID uuid.UUID) error
}

// PixiRunner runs pixi in a project directory (internal/client/sdk/exec).
//
// Each method sends pixi's stdout and stderr to out, the job's log, and
// starts pixi with ctx through process.CommandContext, so on unix
// cancelling ctx stops pixi and its child processes and the method returns
// promptly. On other systems only pixi itself is stopped.
type PixiRunner interface {
	// Lock solves the manifest in dir and writes dir/pixi.lock.
	Lock(ctx context.Context, dir string, out io.Writer) error
	// Install materializes the locked environment in dir/.pixi.
	Install(ctx context.Context, dir string, out io.Writer) error
	// CheckLock checks that dir/pixi.lock satisfies dir/pixi.toml, without
	// changing either file. It returns nil for a current lock. A stale lock
	// is reported with an error that implements StaleLockError, which the
	// SDK turns into ErrLockStale; any other error means the check itself
	// could not run.
	CheckLock(ctx context.Context, dir string, out io.Writer) error
}

// StaleLockError is implemented by the error PixiRunner.CheckLock returns
// for a stale lock. The runner (internal/client/sdk/exec) defines its own
// error type with a StaleLock method, so it does not import this package
// and its database code; errors.As finds the error through wrapping.
type StaleLockError interface {
	error
	// StaleLock reports whether the lock is stale.
	StaleLock() bool
}

// isStaleLock reports whether err, from PixiRunner.CheckLock, reports a
// stale lock.
func isStaleLock(err error) bool {
	var stale StaleLockError
	return errors.As(err, &stale) && stale.StaleLock()
}

// DeleteHook deletes rows another part of the client keeps for a project
// (for example local publications or server links), inside the transaction
// that deletes the project row.
type DeleteHook func(tx *gorm.DB, projectID uuid.UUID) error

// Options configures a Service. DB, Jobs and Pixi are required.
type Options struct {
	DB   *gorm.DB
	Jobs JobRunner
	Pixi PixiRunner

	// ProjectsDir is where projects go when no path is given. Empty means
	// ~/nebi-projects. The SDK reads no environment variables; entrypoints
	// that honour NEBI_PROJECTS_DIR or a flag pass the result here.
	ProjectsDir string

	// SizeTTL is how long a computed project size is reused before List
	// recomputes it in the background. Zero means one minute.
	SizeTTL time.Duration

	// Author is recorded as the author of the versions this service
	// creates. Empty means the operating system user name, or "unknown".
	Author string

	// DeleteHooks run inside DeleteProject's transaction, after the
	// project's versions are deleted.
	DeleteHooks []DeleteHook
}

// Service is the local project SDK. Call Close when done with it.
type Service struct {
	db          *gorm.DB
	jobs        JobRunner
	pixi        PixiRunner
	projectsDir string
	sizeTTL     time.Duration
	deleteHooks []DeleteHook
	author      string

	// mu serializes the operations that write project rows, so the
	// checks they run and the writes they make cannot interleave.
	mu sync.Mutex
	// versionMu serializes version and tag writes; see versionTx.
	versionMu sync.Mutex

	sizes sizeCache
	// measureDir and now are replaced in tests.
	measureDir func(ctx context.Context, dir string) (int64, error)
	now        func() time.Time

	bgCtx    context.Context
	bgCancel context.CancelFunc
	bg       sync.WaitGroup
}

// New returns a Service. The client tables must exist; see Migrate. Call
// RecoverDeletes and RecoverInPlaceSolves once at startup to finish file
// removals and in-place solves that an earlier run left incomplete.
func New(opts Options) (*Service, error) {
	switch {
	case opts.DB == nil:
		return nil, errors.New("local: Options.DB is required")
	case opts.Jobs == nil:
		return nil, errors.New("local: Options.Jobs is required")
	case opts.Pixi == nil:
		return nil, errors.New("local: Options.Pixi is required")
	}
	ttl := opts.SizeTTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		db:          opts.DB,
		jobs:        opts.Jobs,
		pixi:        opts.Pixi,
		projectsDir: opts.ProjectsDir,
		sizeTTL:     ttl,
		deleteHooks: opts.DeleteHooks,
		author:      versionAuthor(opts.Author),
		sizes:       newSizeCache(),
		measureDir:  measureDir,
		now:         time.Now,
		bgCtx:       ctx,
		bgCancel:    cancel,
	}, nil
}

// Close stops background size computation and waits for it to return. It
// does not close the JobRunner, which the caller owns.
func (s *Service) Close() {
	s.bgCancel()
	s.bg.Wait()
}

// versionAuthor returns author, or the operating system user name when it
// is empty, or "unknown".
func versionAuthor(author string) string {
	if author = strings.TrimSpace(author); author != "" {
		return author
	}
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
		return u.Username
	}
	return "unknown"
}
