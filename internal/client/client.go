// Package client is the composition root of the local client stack
// (issue #649): it wires the SDK (internal/client/sdk), the job system
// (internal/client/jobs), host capabilities (internal/client/desktop) and
// the client REST API (internal/client/api).
//
// Three hosts use it:
//   - nebi-web: cmd/nebi-web passes NewExtension(desktop.Capabilities{}) to
//     server.Config.Local; jobs run on the shared worker.
//   - desktop: app.go passes NewExtension with a FolderPicker.
//   - CLI: cmd/nebi-cli calls OpenCLI; jobs run inline.
//
// The bootstrap packages (internal/server, internal/api) only see the
// localext.Extension interface and never import this package.
//
// This package is frozen.
package client

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/gin-gonic/gin"
	clientapi "github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/localext"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
)

// Stack is a wired client stack.
type Stack struct {
	SDK     *sdk.Client
	Jobs    *jobs.Service
	Desktop desktop.Capabilities
}

// StackOptions configures NewStack.
type StackOptions struct {
	DB          *gorm.DB
	Executor    executor.Executor
	DataDir     string
	ProjectsDir string
	// Queue: worker queue; nil runs jobs inline (CLI).
	Queue *queue.MemoryQueue
	// InlineLog receives inline job logs (CLI).
	InlineLog io.Writer
	Desktop   desktop.Capabilities
	Logger    *slog.Logger
}

// NewStack wires the SDK and job service on an already migrated database.
func NewStack(opts StackOptions) (*Stack, error) {
	c, err := sdk.New(shared.Deps{
		DB:          opts.DB,
		Executor:    opts.Executor,
		DataDir:     opts.DataDir,
		ProjectsDir: opts.ProjectsDir,
		Logger:      opts.Logger,
	})
	if err != nil {
		return nil, err
	}
	j := jobs.New(c, jobs.Options{Queue: opts.Queue, InlineLog: opts.InlineLog, Logger: opts.Logger})
	return &Stack{SDK: c, Jobs: j, Desktop: opts.Desktop}, nil
}

// API returns the client REST API over the stack.
func (s *Stack) API() *clientapi.API {
	return clientapi.New(s.SDK, s.Jobs, s.Desktop)
}

// Extension adapts a Stack to localext.Extension for nebi-web and desktop.
type Extension struct {
	desktop desktop.Capabilities
	stack   *Stack
}

var _ localext.Extension = (*Extension)(nil)

// NewExtension returns the local-mode extension with the given host
// capabilities (empty for nebi-web).
func NewExtension(caps desktop.Capabilities) *Extension {
	return &Extension{desktop: caps}
}

// Stack returns the stack after Init (nil before).
func (e *Extension) Stack() *Stack { return e.stack }

// Init migrates the client tables and builds the stack.
func (e *Extension) Init(deps localext.Deps) error {
	if err := local.Migrate(deps.DB); err != nil {
		return fmt.Errorf("client migrations: %w", err)
	}
	dataDir, err := dataDirFor(deps)
	if err != nil {
		return err
	}
	projectsDir := ""
	if deps.Config != nil && deps.Config.Storage.ProjectsDir != "" {
		projectsDir, _ = filepath.Abs(deps.Config.Storage.ProjectsDir)
	}
	stack, err := NewStack(StackOptions{
		DB:          deps.DB,
		Executor:    deps.Executor,
		DataDir:     dataDir,
		ProjectsDir: projectsDir,
		Queue:       deps.Queue,
		Desktop:     e.desktop,
		Logger:      deps.Logger,
	})
	if err != nil {
		return err
	}
	e.stack = stack
	return nil
}

// Dispatcher returns the client job dispatcher for the worker.
func (e *Extension) Dispatcher() localext.Dispatcher {
	return e.stack.Jobs.Dispatcher()
}

// BindWorker hands the worker to the job service.
func (e *Extension) BindWorker(w localext.Worker) {
	e.stack.Jobs.BindWorker(w)
}

// RegisterRoutes mounts /api/v1/client.
func (e *Extension) RegisterRoutes(protected *gin.RouterGroup) {
	e.stack.API().Register(protected)
}

// dataDirFor is the directory of the SQLite database, or the default data
// dir for other drivers.
func dataDirFor(deps localext.Deps) (string, error) {
	if deps.Config != nil && deps.Config.Database.Driver == "sqlite" && deps.Config.Database.DSN != "" {
		return filepath.Abs(filepath.Dir(deps.Config.Database.DSN))
	}
	return store.DefaultDataDir()
}
