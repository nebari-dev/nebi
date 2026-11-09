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
// Options.InlineLog and the job row.
func (s *Service) runInline(ctx context.Context, job *models.Job) (*models.Job, error) {
	db := s.sdk.Deps().DB
	now := time.Now()
	job.Status = models.JobStatusRunning
	job.StartedAt = &now
	db.Save(job)

	var buf bytes.Buffer
	var mu sync.Mutex
	var out io.Writer = &lockedWriter{w: &buf, mu: &mu}
	if s.opts.InlineLog != nil {
		out = io.MultiWriter(out, s.opts.InlineLog)
	}
	runErr := s.dispatcher.Run(ctx, job, out)

	done := time.Now()
	job.CompletedAt = &done
	mu.Lock()
	job.Logs = buf.String()
	mu.Unlock()
	switch {
	case runErr == nil:
		job.Status = models.JobStatusCompleted
	case errors.Is(runErr, context.Canceled) || ctx.Err() != nil:
		job.Status = models.JobStatusCancelled
		job.Error = "Cancelled"
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

type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
