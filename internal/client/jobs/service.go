package jobs

// Owner: U8 (jobs). Jobs page backend: list with filters and sort, detail,
// cancel (real context cancellation through the worker, status
// "cancelled"), live log stream, install-failed linkage.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/localext"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/nebari-dev/nebi/internal/worker"
	"gorm.io/gorm"
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

	// pollInterval paces DB polling: log follow for jobs running in
	// another process, inline cancel watch and inline log flush.
	pollInterval time.Duration
	// cancelWait bounds how long Cancel waits for a running job to settle.
	cancelWait time.Duration
}

// New builds the job service and installs it as the SDK's job submitter
// and as the FailedInstallJobID project decorator.
func New(c *sdk.Client, opts Options) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Service{
		sdk:          c,
		dispatcher:   NewDispatcher(c),
		opts:         opts,
		pollInterval: 500 * time.Millisecond,
		cancelWait:   5 * time.Second,
	}
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

func (s *Service) db(ctx context.Context) *gorm.DB {
	return s.sdk.Deps().DB.WithContext(ctx)
}

func (s *Service) jobRows() *service.JobService {
	return service.NewJobService(s.sdk.Deps().DB, true)
}

// jobRow is a job joined with its project's name. Soft-deleted projects
// still give their name so the Jobs page can show it after a delete.
type jobRow struct {
	models.Job
	ProjectName string
}

// selectJobs is the base query for job rows with project names.
func (s *Service) selectJobs(ctx context.Context) *gorm.DB {
	return s.db(ctx).
		Model(&models.Job{}).
		Select("jobs.*, projects.name AS project_name").
		Joins("LEFT JOIN projects ON projects.id = jobs.project_id")
}

// List returns jobs matching the filter: every job on this machine (client
// and legacy jobs alike), newest start first unless Sort is "started_at".
// Jobs that have not started sort by their creation time.
func (s *Service) List(ctx context.Context, filter shared.JobListFilter) ([]shared.JobSummary, error) {
	order, err := sortOrder(filter.Sort)
	if err != nil {
		return nil, err
	}
	for _, st := range filter.Statuses {
		if !validStatus(st) {
			return nil, &shared.ValidationError{Field: "status", Message: fmt.Sprintf("unknown job status %q", st)}
		}
	}
	for _, t := range filter.Types {
		if _, ok := models.JobTypeLabels[t]; !ok {
			return nil, &shared.ValidationError{Field: "type", Message: fmt.Sprintf("unknown job type %q", t)}
		}
	}
	if filter.Limit < 0 {
		return nil, &shared.ValidationError{Field: "limit", Message: "limit must not be negative"}
	}

	q := s.selectJobs(ctx)
	if len(filter.Statuses) > 0 {
		q = q.Where("jobs.status IN ?", filter.Statuses)
	}
	if len(filter.Types) > 0 {
		q = q.Where("jobs.type IN ?", filter.Types)
	}
	if filter.ProjectID != nil {
		q = q.Where("jobs.project_id = ?", *filter.ProjectID)
	}
	q = q.Order(order)
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	var rows []jobRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	out := make([]shared.JobSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, shared.JobSummaryFrom(r.Job, r.ProjectName))
	}
	return out, nil
}

// sortOrder maps the Sort filter to an ORDER BY clause.
func sortOrder(sort string) (string, error) {
	switch sort {
	case "", "-started_at":
		return "COALESCE(jobs.started_at, jobs.created_at) DESC, jobs.created_at DESC", nil
	case "started_at":
		return "COALESCE(jobs.started_at, jobs.created_at) ASC, jobs.created_at ASC", nil
	default:
		return "", &shared.ValidationError{Field: "sort", Message: fmt.Sprintf("unknown sort %q (use started_at or -started_at)", sort)}
	}
}

func validStatus(st models.JobStatus) bool {
	switch st {
	case models.JobStatusPending, models.JobStatusRunning, models.JobStatusCompleted,
		models.JobStatusFailed, models.JobStatusCancelled:
		return true
	}
	return false
}

// load returns one job row with its project name.
func (s *Service) load(ctx context.Context, id uuid.UUID) (*jobRow, error) {
	var rows []jobRow
	if err := s.selectJobs(ctx).Where("jobs.id = ?", id).Limit(1).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load job: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("job %s: %w", id, shared.ErrNotFound)
	}
	return &rows[0], nil
}

// Get returns one job with its log.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*shared.JobDetail, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	return &shared.JobDetail{JobSummary: shared.JobSummaryFrom(row.Job, row.ProjectName), Logs: row.Logs}, nil
}

// ResolveJobID resolves a full job UUID or a unique prefix of one (the CLI
// shows the first 8 characters).
func (s *Service) ResolveJobID(ctx context.Context, ref string) (uuid.UUID, error) {
	ref = strings.ToLower(strings.TrimSpace(ref))
	if id, err := uuid.Parse(ref); err == nil {
		return id, nil
	}
	if len(ref) < 4 || strings.Trim(ref, "0123456789abcdef-") != "" {
		return uuid.Nil, &shared.ValidationError{Field: "job", Message: fmt.Sprintf("invalid job id %q: use the full id or at least 4 characters of it", ref)}
	}
	var ids []string
	if err := s.db(ctx).Model(&models.Job{}).Where("id LIKE ?", ref+"%").Limit(2).Pluck("id", &ids).Error; err != nil {
		return uuid.Nil, fmt.Errorf("resolve job: %w", err)
	}
	switch len(ids) {
	case 0:
		return uuid.Nil, fmt.Errorf("job %q: %w", ref, shared.ErrNotFound)
	case 1:
		return uuid.Parse(ids[0])
	default:
		return uuid.Nil, &shared.ValidationError{Field: "job", Message: fmt.Sprintf("job id %q is ambiguous; use more characters", ref)}
	}
}

// Cancel cancels a pending or running job. Running jobs are cancelled
// through the worker (context cancellation of the job and its pixi
// process, status "cancelled"); pending jobs are marked cancelled and
// skipped by the worker. A running job owned by another process (a CLI
// inline job) is marked cancelled in the database; that process watches
// its row and stops the job. Cancelling a cancelled job is a no-op;
// cancelling a finished one is a conflict ("job_finished").
func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (*shared.JobSummary, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	switch row.Status {
	case models.JobStatusCancelled:
		sum := shared.JobSummaryFrom(row.Job, row.ProjectName)
		return &sum, nil
	case models.JobStatusCompleted, models.JobStatusFailed:
		return nil, &shared.ConflictError{Code: "job_finished", Message: "This job already finished."}
	}

	if row.Status == models.JobStatusRunning {
		if w := s.boundWorker(); w != nil && w.CancelJob(id) {
			return s.awaitSettled(ctx, id)
		}
	}
	if _, err := s.jobRows().CancelActive(id); err != nil {
		return nil, err
	}
	// Re-read: the job may have finished between the load and the update.
	row, err = s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	switch row.Status {
	case models.JobStatusCompleted, models.JobStatusFailed:
		return nil, &shared.ConflictError{Code: "job_finished", Message: "This job already finished."}
	}
	sum := shared.JobSummaryFrom(row.Job, row.ProjectName)
	return &sum, nil
}

// awaitSettled waits (bounded by cancelWait) for a job cancelled through
// the worker to reach a final state, so the response usually reports
// "cancelled". It returns the latest row either way.
func (s *Service) awaitSettled(ctx context.Context, id uuid.UUID) (*shared.JobSummary, error) {
	deadline := time.Now().Add(s.cancelWait)
	for {
		row, err := s.load(ctx, id)
		if err != nil {
			return nil, err
		}
		if !isActive(row.Status) || time.Now().After(deadline) {
			sum := shared.JobSummaryFrom(row.Job, row.ProjectName)
			return &sum, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func isActive(st models.JobStatus) bool {
	return st == models.JobStatusPending || st == models.JobStatusRunning
}

// stopGrace bounds how long a cancelled job counts as still stopping.
const stopGrace = 2 * time.Minute

// stillStopping reports whether a started job was marked cancelled by
// another process (the row says cancelled) while the process running it
// is still stopping: its stored log does not end with the cancel line
// yet. FollowLogs keeps following such a job so the cleanup output and
// the final line are not lost. Bounded by stopGrace after the last status
// write, in case the runner died.
func stillStopping(job models.Job, now time.Time) bool {
	if job.Status != models.JobStatusCancelled || job.StartedAt == nil || job.CompletedAt == nil {
		return false
	}
	if strings.HasSuffix(strings.TrimRight(job.Logs, "\n"), worker.CancelledLogLine) {
		return false
	}
	return now.Sub(*job.CompletedAt) < stopGrace
}

// ErrStreamClosed is returned by FollowLogs when emit fails (the reader
// went away).
var ErrStreamClosed = errors.New("log stream closed")

// FollowLogs writes a job's log to emit as it grows and returns the job's
// final summary once it ends. A finished job's stored log is emitted in one
// call. A job running in this process is followed through the worker's log
// broker (history first, then live chunks, each byte exactly once); a job
// queued here or running in another process (CLI) is followed by polling
// its stored log. Returns ctx.Err() when ctx ends first.
func (s *Service) FollowLogs(ctx context.Context, id uuid.UUID, emit func(chunk string) error) (*shared.JobSummary, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	sent := 0
	send := func(full string) error {
		if len(full) <= sent {
			return nil
		}
		chunk := full[sent:]
		sent = len(full)
		if err := emit(chunk); err != nil {
			return fmt.Errorf("%w: %w", ErrStreamClosed, err)
		}
		return nil
	}

	for {
		if !isActive(row.Status) && !stillStopping(row.Job, time.Now()) {
			if err := send(row.Logs); err != nil {
				return nil, err
			}
			sum := shared.JobSummaryFrom(row.Job, row.ProjectName)
			return &sum, nil
		}
		if broker := s.Broker(); broker != nil {
			if history, ch, ok := broker.SubscribeWithHistory(id); ok {
				if err := send(history); err != nil {
					broker.Unsubscribe(id, ch)
					return nil, err
				}
				return s.followBroker(ctx, id, broker, ch, emit)
			}
		}
		if err := send(row.Logs); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.pollInterval):
		}
		if row, err = s.load(ctx, id); err != nil {
			return nil, err
		}
	}
}

// followBroker streams the live chunks of a job running in this process
// after its history was sent (the stored log is a prefix of the broker
// history, so only the unsent tail of the history was emitted).
func (s *Service) followBroker(ctx context.Context, id uuid.UUID, broker *logstream.LogBroker, ch chan string, emit func(string) error) (*shared.JobSummary, error) {
	defer broker.Unsubscribe(id, ch)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case chunk, ok := <-ch:
			if !ok {
				row, err := s.load(context.WithoutCancel(ctx), id)
				if err != nil {
					return nil, err
				}
				sum := shared.JobSummaryFrom(row.Job, row.ProjectName)
				return &sum, nil
			}
			if err := emit(chunk); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrStreamClosed, err)
			}
		}
	}
}
