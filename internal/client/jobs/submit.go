package jobs

// Owner: U8 (jobs). Submit implements shared.JobSubmitter.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

// Submit records the job (origin=client) and enqueues it on the worker
// queue, or runs it inline when the service has no queue (CLI). The
// returned job is the stored row; inline jobs are returned finished.
func (s *Service) Submit(ctx context.Context, req shared.JobRequest) (*models.Job, error) {
	if req.ProjectID == uuid.Nil {
		return nil, &shared.ValidationError{Field: "project_id", Message: "job needs a project"}
	}
	metadata := map[string]any{}
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	metadata[MetadataOrigin] = OriginClient
	job := &models.Job{
		ProjectID:     req.ProjectID,
		UserID:        s.sdk.Deps().LocalUserID,
		Type:          req.Type,
		Status:        models.JobStatusPending,
		Metadata:      metadata,
		VersionNumber: req.VersionNumber,
	}
	db := s.sdk.Deps().DB.WithContext(ctx)
	if err := db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	if s.opts.Queue == nil {
		return s.runInline(ctx, job)
	}
	if err := s.opts.Queue.Enqueue(ctx, job); err != nil {
		now := time.Now()
		job.Status = models.JobStatusFailed
		job.Error = err.Error()
		job.CompletedAt = &now
		s.sdk.Deps().DB.Save(job)
		return nil, fmt.Errorf("enqueue job: %w", err)
	}
	return job, nil
}

// runInline runs a job in the calling goroutine (CLI), writing its log to
// Options.InlineLog and the job row. The stored log is flushed every
// pollInterval so the desktop app or nebi-web can show it live, and the
// row is watched so a cancel from another process (the Jobs page, "nebi
// job cancel") stops the job. Ctrl-C cancels ctx and so the job.
func (s *Service) runInline(ctx context.Context, job *models.Job) (*models.Job, error) {
	db := s.sdk.Deps().DB
	rows := s.jobRows()
	if !rows.StartJob(job) {
		return job, context.Canceled
	}

	var buf bytes.Buffer
	var mu sync.Mutex
	var out io.Writer = &lockedWriter{w: &buf, mu: &mu}
	if s.opts.InlineLog != nil {
		out = io.MultiWriter(out, s.opts.InlineLog)
	}
	snapshot := func() string {
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		s.watchInline(runCtx, job.ID, snapshot, cancel)
	}()

	runErr := s.dispatcher.Run(runCtx, job, out)
	cancelled := errors.Is(context.Cause(runCtx), errCancelledElsewhere) ||
		ctx.Err() != nil ||
		(runErr != nil && errors.Is(runErr, context.Canceled))
	cancel(nil)
	<-watchDone

	done := time.Now()
	job.CompletedAt = &done
	job.Logs = snapshot()
	switch {
	case cancelled:
		job.Status = models.JobStatusCancelled
		job.Error = "Cancelled"
		if runErr == nil {
			runErr = context.Canceled
		}
	case runErr == nil:
		job.Status = models.JobStatusCompleted
	default:
		job.Status = models.JobStatusFailed
		job.Error = runErr.Error()
	}
	// Persist even when ctx was cancelled.
	db.WithContext(context.WithoutCancel(ctx)).Save(job)
	if runErr != nil {
		return job, runErr
	}
	return job, nil
}

// errCancelledElsewhere is the cause when another process cancelled an
// inline job through its row.
var errCancelledElsewhere = errors.New("job cancelled from another process")

// watchInline flushes the inline job's log to its row and cancels the job
// when the row was marked cancelled, until ctx ends.
func (s *Service) watchInline(ctx context.Context, id uuid.UUID, snapshot func() string, cancel context.CancelCauseFunc) {
	rows := s.jobRows()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	flushed := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if logs := snapshot(); len(logs) != flushed {
				if err := rows.FlushLogs(id, logs); err == nil {
					flushed = len(logs)
				}
			}
			if rows.IsCancelled(id) {
				cancel(errCancelledElsewhere)
				return
			}
		}
	}
}

type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
