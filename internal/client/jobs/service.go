package jobs

// Owner: U8 (jobs). Jobs page backend: list with filters and sort, detail,
// cancel (real context cancellation through the worker, status
// "cancelled"), live log stream, install-failed linkage.

import (
	"context"
	"io"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/localext"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/queue"
)

// Options configures the job service.
type Options struct {
	// Queue is the worker queue (nebi-web, desktop). Nil runs jobs inline
	// in Submit (CLI).
	Queue *queue.MemoryQueue
	// InlineLog receives inline job logs (CLI: the terminal). May be nil.
	InlineLog io.Writer
	Logger    *slog.Logger
}

// Service is the client job service.
type Service struct {
	sdk        *sdk.Client
	dispatcher *Dispatcher
	opts       Options

	mu     sync.RWMutex
	worker localext.Worker
}

// New builds the job service and installs it as the SDK's job submitter
// and as the FailedInstallJobID project decorator.
func New(c *sdk.Client, opts Options) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Service{sdk: c, dispatcher: NewDispatcher(c), opts: opts}
	c.Runtime().SetJobs(s)
	c.Runtime().AddProjectDecorator(s)
	return s
}

// Dispatcher returns the dispatcher to hand to the worker.
func (s *Service) Dispatcher() *Dispatcher { return s.dispatcher }

// BindWorker gives the service the worker (cancellation, log broker).
func (s *Service) BindWorker(w localext.Worker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worker = w
}

// boundWorker returns the bound worker (nil in the CLI).
func (s *Service) boundWorker() localext.Worker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.worker
}

// Broker returns the worker's log broker for live streams (nil in the CLI).
func (s *Service) Broker() *logstream.LogBroker {
	if w := s.boundWorker(); w != nil {
		return w.GetBroker()
	}
	return nil
}

// List returns jobs matching the filter.
func (s *Service) List(ctx context.Context, filter shared.JobListFilter) ([]shared.JobSummary, error) {
	return nil, shared.NotImplemented("jobs.List")
}

// Get returns one job with its log.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*shared.JobDetail, error) {
	return nil, shared.NotImplemented("jobs.Get")
}

// Cancel cancels a pending or running job. Running jobs are cancelled
// through the worker (context cancellation, status "cancelled"); pending
// jobs are marked cancelled and skipped by the worker.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (*shared.JobSummary, error) {
	return nil, shared.NotImplemented("jobs.Cancel")
}

// DecorateProjects fills FailedInstallJobID.
func (s *Service) DecorateProjects(ctx context.Context, items []shared.ProjectListItem) error {
	return nil
}
