// Package local is the client SDK for projects on this machine (issue
// #660): the local Project data model and its create, list, get, rename and
// delete operations. Everything here runs on the user's machine; the server
// is never contacted, and a project exists on a server only after it is
// pushed.
//
// The SDK owns its tables in the client database (nebi.db) and shares none
// with the server models in internal/models.
//
// Work that belongs to other parts of the client is reached through small
// interfaces, so each part can be built and tested on its own:
//   - VersionStore: project versions and tags (the version CRUD issue, #661;
//     a temporary seam removed when #661 lands, tracked in #669)
//   - JobRunner: background jobs (internal/client/jobs)
//   - PixiRunner: pixi lock and install (internal/client/sdk/exec)
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

// Temporary seam until the version CRUD issue (#661) lands. #661 specifies
// recordVersion as an unexported method in this package,
// func (s *Service) recordVersion(tx, req) (*ProjectVersion, bool, error),
// so VersionStore, VersionOrigin, OriginCreated and RecordVersionRequest
// are placeholders that will be removed or collide with #661's own types
// when it lands. Replacing them with the real version tables is tracked in
// #669.

// VersionOrigin records how a version entered this machine's history. The
// version CRUD issue (#661) owns it and adds the other origins. Temporary;
// see the note above.
type VersionOrigin string

// OriginCreated is the origin of a version made on this machine, including
// version 1, which the create job records.
const OriginCreated VersionOrigin = "created"

// RecordVersionRequest mirrors recordVersionRequest from the version CRUD
// issue (#661): everything recordVersion needs to write one version.
// Temporary; #661 replaces it with its own unexported request (#669).
type RecordVersionRequest struct {
	ProjectID uuid.UUID
	// BaseVersionID is the version this one was created from; nil for
	// version 1.
	BaseVersionID *uuid.UUID
	// Manifest and Lock are the complete pixi.toml and pixi.lock.
	Manifest string
	Lock     string
	Origin   VersionOrigin
	Author   string
	// Description is the version's description, not the workspace's.
	Description string
	Tags        []string
	// DedupeLatest returns the latest version instead of inserting a new
	// one when its content hash matches.
	DedupeLatest bool
}

// VersionInfo is the version data the project list shows.
type VersionInfo struct {
	// Count is the number of versions of the project.
	Count int
	// Latest is the newest version number; nil when there are no versions.
	Latest *int
	// Installed is the number of the installed version; nil when nothing
	// is installed.
	Installed *int
}

// VersionStore is the version side of a project, owned by the version CRUD
// issue (#661), which provides the ProjectVersion and ProjectTag tables and
// recordVersion.
//
// Temporary seam: #661 makes recordVersion an unexported Service method in
// this package, so this interface and the fake behind it go away when #661
// lands. The cleanup is tracked in #669.
type VersionStore interface {
	// RecordVersion is #661's recordVersion: it validates the tags and
	// writes the version and its tags inside tx, the job's success
	// transaction, and returns the version's ID.
	RecordVersion(tx *gorm.DB, req RecordVersionRequest) (uuid.UUID, error)
	// VersionInfo reports the version count, the latest version number and
	// the number of the version whose ID is installedID (nil: none).
	VersionInfo(ctx context.Context, projectID uuid.UUID, installedID *uuid.UUID) (VersionInfo, error)
	// DeleteProjectVersions deletes every version and tag of the project
	// inside tx, the transaction that deletes the project row.
	DeleteProjectVersions(tx *gorm.DB, projectID uuid.UUID) error
}

// JobRunner submits and reports background jobs (internal/client/jobs).
// *jobs.Memory implements it.
type JobRunner interface {
	// Submit adds run as a job of the given type for the project. The job
	// is running when it could start at once, and pending while it waits
	// for a free slot or for the project's running job to finish.
	Submit(ctx context.Context, projectID uuid.UUID, typ jobs.Type, run jobs.Func) (*jobs.Job, error)
	// ProjectJobs returns the project's jobs the runner still keeps,
	// oldest first: every pending or running job, the latest finished one
	// while the project exists, and older finished ones only until they
	// are dropped.
	ProjectJobs(ctx context.Context, projectID uuid.UUID) ([]jobs.Job, error)
	// RetireProject atomically checks that the project has no pending or
	// running job and, when it has none, refuses every later job for it
	// except the allowed types. When jobs are active it returns their IDs
	// and changes nothing.
	RetireProject(ctx context.Context, projectID uuid.UUID, allow ...jobs.Type) ([]uuid.UUID, error)
	// RestoreProject undoes RetireProject.
	RestoreProject(ctx context.Context, projectID uuid.UUID) error
	// Cancel cancels a job: a pending job ends as cancelled without
	// running, and a running job has its context cancelled. Cancelling a
	// finished job does nothing. A file removal job cannot be cancelled;
	// for one that is active, Cancel returns a *jobs.NotCancellableError.
	Cancel(ctx context.Context, jobID uuid.UUID) error
}

// PixiRunner runs pixi in a project directory (internal/client/sdk/exec).
//
// Each method sends pixi's stdout and stderr to out, the job's log, and
// starts pixi with ctx through process.CommandContext, so cancelling ctx
// stops pixi and its child processes and the method returns promptly.
type PixiRunner interface {
	// Lock solves the manifest in dir and writes dir/pixi.lock.
	Lock(ctx context.Context, dir string, out io.Writer) error
	// Install materializes the locked environment in dir/.pixi.
	Install(ctx context.Context, dir string, out io.Writer) error
}

// DeleteHook deletes rows another part of the client keeps for a project
// (for example local publications or server links), inside the transaction
// that deletes the project row.
type DeleteHook func(tx *gorm.DB, projectID uuid.UUID) error

// Options configures a Service. DB, Versions, Jobs and Pixi are required.
type Options struct {
	DB       *gorm.DB
	Versions VersionStore
	Jobs     JobRunner
	Pixi     PixiRunner

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
	versions    VersionStore
	jobs        JobRunner
	pixi        PixiRunner
	projectsDir string
	sizeTTL     time.Duration
	deleteHooks []DeleteHook
	author      string

	// mu serializes the operations that write project rows, so the
	// checks they run and the writes they make cannot interleave.
	mu sync.Mutex

	sizes sizeCache
	// measureDir and now are replaced in tests.
	measureDir func(ctx context.Context, dir string) (int64, error)
	now        func() time.Time

	bgCtx    context.Context
	bgCancel context.CancelFunc
	bg       sync.WaitGroup
}

// New returns a Service. The client tables must exist; see Migrate. Call
// RecoverDeletes once at startup to finish file removals that an earlier
// run left incomplete.
func New(opts Options) (*Service, error) {
	switch {
	case opts.DB == nil:
		return nil, errors.New("local: Options.DB is required")
	case opts.Versions == nil:
		return nil, errors.New("local: Options.Versions is required")
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
		versions:    opts.Versions,
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
