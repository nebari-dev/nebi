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
//   - VersionStore: project versions and tags (the version CRUD issue, #661)
//   - JobRunner: background jobs (internal/client/jobs)
//   - PixiRunner: pixi lock and install (internal/client/sdk/exec)
package local

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
)

// VersionOriginCreated is the origin of version 1, recorded by the create job.
const VersionOriginCreated = "created"

// RecordVersionInput is what the create job hands to VersionStore.RecordVersion.
type RecordVersionInput struct {
	ProjectID uuid.UUID
	// Manifest and Lock are the contents of pixi.toml and pixi.lock.
	Manifest string
	Lock     string
	// Origin says how the version came to be; VersionOriginCreated for version 1.
	Origin string
	// BaseVersionID is the version this one was derived from; nil for version 1.
	BaseVersionID *uuid.UUID
	Tags          []string
	// JobID is the job that produced the version, when there is one.
	JobID *uuid.UUID
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
type VersionStore interface {
	// RecordVersion writes a new immutable version and its tags and returns
	// the version's ID.
	RecordVersion(ctx context.Context, in RecordVersionInput) (uuid.UUID, error)
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
	// Submit starts run as a job of the given type for the project.
	Submit(ctx context.Context, projectID uuid.UUID, typ jobs.Type, run jobs.Func) (*jobs.Job, error)
	// ProjectJobs returns the project's jobs, oldest first.
	ProjectJobs(ctx context.Context, projectID uuid.UUID) ([]jobs.Job, error)
}

// PixiRunner runs pixi in a project directory (internal/client/sdk/exec).
type PixiRunner interface {
	// Lock solves the manifest in dir and writes dir/pixi.lock.
	Lock(ctx context.Context, dir string) error
	// Install materializes the locked environment in dir/.pixi.
	Install(ctx context.Context, dir string) error
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
	// $NEBI_PROJECTS_DIR, or ~/nebi-projects when that is unset too.
	ProjectsDir string

	// SizeTTL is how long a computed project size is reused before List
	// recomputes it in the background. Zero means one minute.
	SizeTTL time.Duration

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

// New returns a Service. The client tables must exist; see Migrate.
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
