// Package localext is the contract between the shared bootstrap
// (internal/server.Run for nebi-web, app.go for the desktop app, and the
// router in internal/api) and the local-mode client stack in
// internal/client (issue #649).
//
// The bootstrap packages never import internal/client: the executable's
// main package builds an Extension (internal/client.NewExtension) and hands
// it over through server.Config.Local or api.RouterOptions.Local. Team mode
// never sets it, so the team server carries no client code.
package localext

import (
	"context"
	"io"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"gorm.io/gorm"
)

// Deps are the bootstrap-owned resources the client stack is built on.
type Deps struct {
	Config   *config.Config
	DB       *gorm.DB
	Queue    *queue.MemoryQueue
	Executor executor.Executor
	Logger   *slog.Logger
}

// Dispatcher matches worker.Dispatcher; the worker runs jobs it Handles.
type Dispatcher interface {
	Handles(job *models.Job) bool
	Run(ctx context.Context, job *models.Job, logWriter io.Writer) error
}

// Worker is the part of *worker.Worker the client stack uses.
type Worker interface {
	CancelJob(jobID uuid.UUID) bool
	GetBroker() *logstream.LogBroker
}

// Extension is the local-mode client stack as seen by the bootstrap.
// Call order: Init, Dispatcher (passed to worker.New), BindWorker,
// RegisterRoutes (from api.NewRouter, local mode only).
type Extension interface {
	// Init builds the stack on the bootstrap's database (after db.Migrate)
	// and runs the client's own migrations.
	Init(deps Deps) error
	// Dispatcher returns the job dispatcher the worker consults first.
	Dispatcher() Dispatcher
	// BindWorker hands over the worker for cancellation and log streaming.
	BindWorker(w Worker)
	// RegisterRoutes mounts the client REST API on the authenticated
	// /api/v1 group. Routes live under /api/v1/client.
	RegisterRoutes(protected *gin.RouterGroup)
}
