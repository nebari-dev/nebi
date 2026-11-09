package shared

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
)

// Deps are the resources every SDK service is built with.
type Deps struct {
	// DB is the local database (see local.OpenDB / local.Migrate).
	DB *gorm.DB
	// Executor runs pixi operations against project directories.
	Executor executor.Executor
	// DataDir is the Nebi data directory holding nebi.db.
	DataDir string
	// ProjectsDir is the default location for new projects ("Leave blank
	// to use Nebi's default location"): a project named n goes to
	// ProjectsDir/n. Empty means DataDir/projects.
	ProjectsDir string
	// LocalUserID is the well-known local user (auth.LocalUsername()).
	LocalUserID uuid.UUID
	// Credentials stores secrets (registry passwords, never in SQLite).
	Credentials store.CredentialStore
	// HTTPClient is used for team-server and registry calls.
	HTTPClient *http.Client
	Logger     *slog.Logger
	// Runtime carries late-bound hooks; never nil after sdk.New.
	Runtime *Runtime
}

// DefaultProjectsDir is where projects are created when no path is given.
func (d Deps) DefaultProjectsDir() string {
	if d.ProjectsDir != "" {
		return d.ProjectsDir
	}
	return filepath.Join(d.DataDir, "projects")
}

// Runtime holds hooks installed after the SDK is constructed: the job
// submitter (internal/client/jobs is built on top of the SDK) and the
// decorators other units register to fill cross-unit response fields.
type Runtime struct {
	mu                sync.RWMutex
	jobs              JobSubmitter
	projectDecorators []ProjectDecorator
	versionDecorators []VersionDecorator
}

// SetJobs installs the job submitter.
func (r *Runtime) SetJobs(j JobSubmitter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = j
}

// Jobs returns the job submitter, or a submitter that fails with
// ErrNotImplemented when none is installed.
func (r *Runtime) Jobs() JobSubmitter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.jobs == nil {
		return noJobs{}
	}
	return r.jobs
}

// AddProjectDecorator registers a decorator for project list items.
func (r *Runtime) AddProjectDecorator(d ProjectDecorator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projectDecorators = append(r.projectDecorators, d)
}

// AddVersionDecorator registers a decorator for version items.
func (r *Runtime) AddVersionDecorator(d VersionDecorator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versionDecorators = append(r.versionDecorators, d)
}

// ProjectDecorators returns the registered project decorators in order.
func (r *Runtime) ProjectDecorators() []ProjectDecorator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]ProjectDecorator(nil), r.projectDecorators...)
}

// VersionDecorators returns the registered version decorators in order.
func (r *Runtime) VersionDecorators() []VersionDecorator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]VersionDecorator(nil), r.versionDecorators...)
}
