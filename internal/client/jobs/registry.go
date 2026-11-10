// Package jobs is the client job system (issue #649): it submits jobs for
// the SDK (implementing shared.JobSubmitter), runs them through the shared
// worker (nebi-web, desktop) or inline (CLI), and serves the Jobs page:
// list with filters, detail, cancel and live logs.
//
// Job bodies live in the SDK (Run* methods). Each unit registers the job
// types it owns in its own handlers_<unit>.go file through register, so no
// one edits a central switch. The worker hands a job to this package only
// when it was submitted through the client stack (metadata origin=client),
// so legacy API jobs keep their old behaviour.
//
// File ownership: registry.go and dispatcher.go are frozen;
// handlers_<unit>.go belong to their unit; service.go, submit.go and any
// other file belong to U8.
package jobs

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/models"
)

// MetadataOrigin is the job metadata key marking client-submitted jobs.
const (
	MetadataOrigin = "origin"
	OriginClient   = "client"
)

// RunContext is what a job handler gets.
type RunContext struct {
	SDK *sdk.Client
	Job *models.Job
	// Log receives the job log (streamed to the UI / terminal and stored).
	Log io.Writer
}

// HandlerFunc runs one job. Returning an error fails the job; a cancelled
// ctx (Cancel job) is reported as cancelled by the runner.
type HandlerFunc func(ctx context.Context, rc RunContext) error

var (
	handlersMu sync.RWMutex
	handlers   = map[models.JobType]HandlerFunc{}
)

// register installs the handler for a job type. Call it from init() in a
// unit-owned handlers_<unit>.go file. Registering a type twice panics.
func register(t models.JobType, h HandlerFunc) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	if _, dup := handlers[t]; dup {
		panic(fmt.Sprintf("jobs: duplicate handler for job type %q", t))
	}
	handlers[t] = h
}

// handlerFor returns the handler for a job type.
func handlerFor(t models.JobType) (HandlerFunc, bool) {
	handlersMu.RLock()
	defer handlersMu.RUnlock()
	h, ok := handlers[t]
	return h, ok
}

// RegisteredTypes lists job types with a handler (for tests and the CLI).
func RegisteredTypes() []models.JobType {
	handlersMu.RLock()
	defer handlersMu.RUnlock()
	out := make([]models.JobType, 0, len(handlers))
	for t := range handlers {
		out = append(out, t)
	}
	return out
}
