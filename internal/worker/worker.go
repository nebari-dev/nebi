package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/process"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/service"
)

// Worker processes jobs from the queue
type Worker struct {
	queue           *queue.MemoryQueue
	executor        executor.Executor
	svc             *service.ProjectService
	jobSvc          *service.JobService
	logger          *slog.Logger
	broker          *logstream.LogBroker
	maxParallelJobs int
	jobTimeout      time.Duration
	maxLogBytes     int
}

type autoReinstallFailureError struct {
	err error
}

func (e *autoReinstallFailureError) Error() string {
	return e.err.Error()
}

// New creates a new worker instance
func New(q *queue.MemoryQueue, exec executor.Executor, svc *service.ProjectService, jobSvc *service.JobService, logger *slog.Logger, limitCfg limits.Limits, maxParallelJobs int) *Worker {
	return &Worker{
		queue:           q,
		executor:        exec,
		svc:             svc,
		jobSvc:          jobSvc,
		logger:          logger,
		broker:          logstream.NewBroker(),
		maxParallelJobs: max(1, maxParallelJobs),
		jobTimeout:      limitCfg.JobTimeout(),
		maxLogBytes:     limitCfg.JobLogBytes,
	}
}

// GetBroker returns the log broker for external access (SSE endpoints)
func (w *Worker) GetBroker() *logstream.LogBroker {
	return w.broker
}

// Start begins processing jobs from the queue
func (w *Worker) Start(ctx context.Context) error {
	defer w.broker.Shutdown()
	w.logger.Info("Worker started", "max_parallel_jobs", w.maxParallelJobs)

	var wg sync.WaitGroup
	for range w.maxParallelJobs {
		wg.Go(func() {
			for ctx.Err() == nil {
				// The in-memory queue blocks until work arrives, it closes, or
				// the context is cancelled. There is no remote queue to poll.
				job, err := w.queue.Dequeue(ctx)
				if err != nil || ctx.Err() != nil {
					return
				}
				w.processJob(ctx, job)
			}
		})
	}
	// Keep the broker alive until every job has saved its final status/logs.
	wg.Wait()
	w.logger.Info("All jobs completed, worker stopped")
	return ctx.Err()
}

func (w *Worker) processJob(ctx context.Context, job *models.Job) {
	// Add panic recovery to prevent pod crashes from panics in job processing
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("Panic recovered in processJob", "job_id", job.ID, "panic", r)
			w.jobSvc.MarkPanicked(job, fmt.Sprintf("Job panicked: %v", r))
		}
	}()

	w.logger.Info("Processing job", "job_id", job.ID, "type", job.Type)

	// Update job status to running
	w.jobSvc.MarkRunning(job)

	// Create thread-safe log buffer. Log writes are capped before they reach
	// this buffer or any streaming backend.
	var logBuf bytes.Buffer
	var logMutex sync.Mutex

	// Start periodic log persistence (flush to DB every 2 seconds)
	stopFlushing := make(chan struct{})
	flushDone := make(chan struct{})
	defer func() {
		close(stopFlushing)
		<-flushDone
	}()

	go func() {
		defer close(flushDone)
		w.flushLogsToDatabase(job.ID, &logBuf, &logMutex, stopFlushing)
	}()

	// Close broker subscriptions when job finishes
	defer w.broker.Close(job.ID)

	// Thread-safe writer wrapper
	safeWriter := &threadSafeWriter{writer: &logBuf, mu: &logMutex}

	// Create broker writer for in-memory streaming
	brokerWriter := logstream.NewStreamWriter(job.ID, w.broker, safeWriter)

	logWriter := newCappedLogWriter(brokerWriter, w.maxLogBytes)

	// Execute the job with streaming logs
	jobCtx := ctx
	cancel := func() {}
	if w.jobTimeout > 0 {
		jobCtx, cancel = context.WithTimeout(ctx, w.jobTimeout)
		fmt.Fprintf(logWriter, "Job deadline: %s\n", w.jobTimeout)
	}
	defer cancel()

	err := w.executeJob(jobCtx, job, logWriter)
	cleanupCause := err
	needsCleanup := err != nil
	if err != nil && errors.Is(jobCtx.Err(), context.DeadlineExceeded) {
		if metricErr := w.jobSvc.RecordJobTimeout(); metricErr != nil {
			w.logger.Error("Failed to record job timeout metric", "job_id", job.ID, "error", metricErr)
		}
		w.logger.Warn("Job exceeded wall-clock deadline", "job_id", job.ID, "timeout", w.jobTimeout)
		if cleanupCause == nil {
			cleanupCause = jobCtx.Err()
		}
		err = fmt.Errorf("job exceeded wall-clock timeout of %s", w.jobTimeout)
		needsCleanup = true
	}
	if needsCleanup {
		w.cleanupFailedJobArtifacts(job, cleanupCause, logWriter)
	}

	// Get final logs (thread-safe)
	logMutex.Lock()
	finalLogs := logBuf.String()
	logMutex.Unlock()

	// Update job status
	if err != nil {
		w.logger.Error("Job failed", "job_id", job.ID, "error", err)
		w.jobSvc.MarkFailed(job, finalLogs, err.Error())
		// Publish error to subscribers
		errorMsg := fmt.Sprintf("\n[ERROR] Job failed: %v\n", err)
		w.broker.Publish(job.ID, errorMsg)
	} else {
		w.logger.Info("Job completed", "job_id", job.ID)
		w.jobSvc.MarkCompleted(job, finalLogs)
		// Publish completion to subscribers
		completionMsg := "\n[COMPLETED] Job finished successfully\n"
		w.broker.Publish(job.ID, completionMsg)
	}
}

func (w *Worker) cleanupFailedJobArtifacts(job *models.Job, jobErr error, logWriter io.Writer) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	project, err := w.jobSvc.LoadProject(job.ProjectID)
	if err != nil {
		w.logger.Error("Failed to load project for job cleanup", "job_id", job.ID, "project_id", job.ProjectID, "error", err)
		fmt.Fprintf(logWriter, "Job cleanup skipped: failed to load project: %v\n", err)
		return
	}
	cleanupJobType := job.Type
	var reinstallErr *autoReinstallFailureError
	if errors.As(jobErr, &reinstallErr) {
		cleanupJobType = models.JobTypeEnvInstall
	}
	if err := w.executor.CleanupJobArtifacts(cleanupCtx, project, cleanupJobType, logWriter); err != nil {
		w.logger.Error("Job cleanup failed", "job_id", job.ID, "project_id", project.ID, "error", err)
		fmt.Fprintf(logWriter, "Job cleanup failed: %v\n", err)
	}
}

// flushLogsToDatabase periodically saves accumulated logs to the database
func (w *Worker) flushLogsToDatabase(jobID uuid.UUID, logBuf *bytes.Buffer, logMutex *sync.Mutex, stop chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Read current logs (thread-safe)
			logMutex.Lock()
			currentLogs := logBuf.String()
			logMutex.Unlock()

			// Save to database
			if err := w.jobSvc.FlushLogs(jobID, currentLogs); err != nil {
				w.logger.Error("Failed to flush logs to database", "job_id", jobID, "error", err)
			}

		case <-stop:
			// Final flush before stopping
			logMutex.Lock()
			finalLogs := logBuf.String()
			logMutex.Unlock()

			if err := w.jobSvc.FlushLogs(jobID, finalLogs); err != nil {
				w.logger.Error("Failed final log flush", "job_id", jobID, "error", err)
			}
			return
		}
	}
}

// threadSafeWriter wraps an io.Writer with a mutex for concurrent access
type threadSafeWriter struct {
	writer io.Writer
	mu     *sync.Mutex
}

func (w *threadSafeWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

const logTruncatedMessage = "\n[TRUNCATED] Job log exceeded configured limit\n"
const logTailReserveBytes = 4096

type cappedLogWriter struct {
	dst       io.Writer
	limit     int
	tailLimit int
	tailUsed  int
	written   int
	truncated bool
	mu        sync.Mutex
}

func newCappedLogWriter(dst io.Writer, limit int) io.Writer {
	if limit <= 0 {
		return dst
	}
	tailLimit := limit / 4
	if tailLimit > logTailReserveBytes {
		tailLimit = logTailReserveBytes
	}
	noticeLen := len(logTruncatedMessage)
	if noticeLen > limit {
		noticeLen = limit
	}
	if tailLimit > limit-noticeLen {
		tailLimit = limit - noticeLen
	}
	return &cappedLogWriter{dst: dst, limit: limit, tailLimit: tailLimit}
}

func (w *cappedLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.truncated {
		if isImportantLogMessage(p) {
			if err := w.writeTailLocked(p); err != nil {
				return 0, err
			}
		}
		return len(p), nil
	}

	notice := []byte(logTruncatedMessage)
	if len(notice) > w.limit {
		notice = notice[:w.limit]
	}
	maxPayload := w.limit - len(notice) - w.tailLimit
	if maxPayload < 0 {
		maxPayload = 0
	}
	remaining := maxPayload - w.written
	if remaining < 0 {
		remaining = 0
	}

	if len(p) <= remaining {
		if _, err := w.dst.Write(p); err != nil {
			return 0, err
		}
		w.written += len(p)
		return len(p), nil
	}

	if remaining > 0 {
		if _, err := w.dst.Write(p[:remaining]); err != nil {
			return 0, err
		}
		w.written += remaining
	}
	if len(notice) > 0 {
		if _, err := w.dst.Write(notice); err != nil {
			return 0, err
		}
		w.written += len(notice)
	}
	w.truncated = true
	if isImportantLogMessage(p) {
		tail := p
		if remaining > 0 && remaining < len(p) {
			tail = p[remaining:]
		}
		if err := w.writeTailLocked(tail); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *cappedLogWriter) writeTailLocked(p []byte) error {
	remaining := w.tailLimit - w.tailUsed
	if remaining <= 0 {
		return nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	if len(p) == 0 {
		return nil
	}
	if _, err := w.dst.Write(p); err != nil {
		return err
	}
	w.tailUsed += len(p)
	w.written += len(p)
	return nil
}

func isImportantLogMessage(p []byte) bool {
	return bytes.Contains(p, []byte("[ERROR]")) ||
		bytes.Contains(p, []byte("Project storage limit")) ||
		bytes.Contains(p, []byte("Job cleanup"))
}

func (w *Worker) executeJob(ctx context.Context, job *models.Job, logWriter io.Writer) error {
	// Load project
	project, err := w.jobSvc.LoadProject(job.ProjectID)
	if err != nil {
		return err
	}

	// Prefer the job row's user ID; fall back to legacy metadata for older jobs.
	userID := project.OwnerID
	if job.UserID != uuid.Nil {
		userID = job.UserID
	} else if userIDInterface, ok := job.Metadata["user_id"]; ok {
		if userIDStr, ok := userIDInterface.(string); ok {
			if parsed, err := uuid.Parse(userIDStr); err == nil {
				userID = parsed
			}
		}
	}

	switch job.Type {
	case models.JobTypeCreate:
		w.svc.SetProjectStatus(project.ID, models.ProjectStatusCreating)

		opts := buildCreateProjectOptions(job.Metadata)

		if err := w.executor.CreateProject(ctx, project, logWriter, opts); err != nil {
			w.svc.SetProjectStatus(project.ID, models.ProjectStatusFailed)
			return err
		}

		// Persist the resolved path so the CLI can find the project on disk.
		// Also update project.Path in memory so the subsequent db.Save in UpdateProjectSize
		// does not overwrite the path back to "". Fail the project on a write
		// error so we never reach the "ready with empty path" state this fix exists
		// to prevent.
		if project.Path == "" {
			resolvedPath := w.executor.GetProjectPath(project)
			if err := w.svc.SetProjectPath(project.ID, resolvedPath); err != nil {
				w.logger.Error("failed to persist project path", "project_id", project.ID, "resolved_path", resolvedPath, "error", err)
				w.svc.SetProjectStatus(project.ID, models.ProjectStatusFailed)
				return err
			}
			project.Path = resolvedPath
		}

		// Create version snapshot
		if err := w.createVersionSnapshot(ctx, project, job.ID, userID, "Initial project creation"); err != nil {
			w.svc.SetProjectStatus(project.ID, models.ProjectStatusFailed)
			return err
		}

		// List installed packages and save to database
		if err := w.syncPackagesFromProject(ctx, project, "Failed to sync packages"); err != nil {
			w.svc.SetProjectStatus(project.ID, models.ProjectStatusFailed)
			return err
		}

		w.svc.SetProjectStatus(project.ID, models.ProjectStatusReady)

	case models.JobTypeInstall:
		packages := parsePackagesFromMetadata(job.Metadata)
		if packages == nil {
			return fmt.Errorf("packages not found in job metadata")
		}

		wasInstalled := w.executor.IsEnvInstalled(project)

		if err := w.executor.InstallPackages(ctx, project, packages, logWriter); err != nil {
			return err
		}

		if err := w.createVersionSnapshot(ctx, project, job.ID, userID, fmt.Sprintf("Installed packages: %v", packages)); err != nil {
			return err
		}

		w.svc.SaveInstalledPackages(project.ID, packages)

		if err := w.maybeReinstallEnv(ctx, project, wasInstalled, logWriter); err != nil {
			return err
		}

	case models.JobTypeRemove:
		packages := parsePackagesFromMetadata(job.Metadata)
		if packages == nil {
			return fmt.Errorf("packages not found in job metadata")
		}

		wasInstalled := w.executor.IsEnvInstalled(project)

		if err := w.executor.RemovePackages(ctx, project, packages, logWriter); err != nil {
			return err
		}

		if err := w.createVersionSnapshot(ctx, project, job.ID, userID, fmt.Sprintf("Removed packages: %v", packages)); err != nil {
			return err
		}

		w.svc.DeletePackagesByName(project.ID, packages)

		if err := w.maybeReinstallEnv(ctx, project, wasInstalled, logWriter); err != nil {
			return err
		}

	case models.JobTypeUpdate:
		wasInstalled := w.executor.IsEnvInstalled(project)
		w.svc.SetProjectStatus(project.ID, models.ProjectStatusCreating)

		fmt.Fprintf(logWriter, "Solving environment from current pixi.toml...\n")

		if err := w.executor.SolveEnvironment(ctx, project, logWriter); err != nil {
			w.svc.SetProjectStatus(project.ID, models.ProjectStatusFailed)
			return err
		}

		if err := w.createVersionSnapshot(ctx, project, job.ID, userID, "Solved environment from updated pixi.toml"); err != nil {
			w.svc.SetProjectStatus(project.ID, models.ProjectStatusFailed)
			return err
		}

		if err := w.syncPackagesFromProject(ctx, project, "Failed to sync packages after solve"); err != nil {
			w.svc.SetProjectStatus(project.ID, models.ProjectStatusFailed)
			return err
		}

		w.svc.SetProjectStatus(project.ID, models.ProjectStatusReady)

		if err := w.maybeReinstallEnv(ctx, project, wasInstalled, logWriter); err != nil {
			return err
		}

	case models.JobTypeEnvInstall:
		if err := w.executor.InstallEnvironment(ctx, project, logWriter); err != nil {
			return err
		}
		w.svc.UpdateProjectSize(project)

	case models.JobTypeEnvUninstall:
		if err := w.executor.UninstallEnvironment(ctx, project, logWriter); err != nil {
			return err
		}
		// Size tracks the installed environment; with no environment there
		// is nothing to measure.
		if err := w.svc.ResetProjectSize(project.ID); err != nil {
			w.logger.Error("failed to reset project size", "project_id", project.ID, "error", err)
		}

	case models.JobTypeDelete:
		w.svc.SetProjectStatus(project.ID, models.ProjectStatusDeleting)

		if err := w.executor.DeleteProject(ctx, project, logWriter); err != nil {
			return err
		}

		w.svc.DeleteAllPackages(project.ID)
		w.svc.SoftDeleteProject(project.ID)

	case models.JobTypeRollback:
		versionIDStr, ok := job.Metadata["version_id"].(string)
		if !ok {
			return fmt.Errorf("version_id not found in job metadata")
		}

		versionID, err := uuid.Parse(versionIDStr)
		if err != nil {
			return fmt.Errorf("invalid version_id: %w", err)
		}

		// Fetch version
		version, err := w.jobSvc.LoadVersion(versionID)
		if err != nil {
			return err
		}

		if version.ProjectID != project.ID {
			return fmt.Errorf("version does not belong to this project")
		}

		fmt.Fprintf(logWriter, "Rolling back to version %d\n", version.VersionNumber)

		wasInstalled := w.executor.IsEnvInstalled(project)

		if err := w.executeRollback(ctx, project, version, logWriter); err != nil {
			return err
		}

		if err := w.createVersionSnapshot(ctx, project, job.ID, userID, fmt.Sprintf("Rolled back to snapshot %d", version.VersionNumber)); err != nil {
			return err
		}

		if err := w.syncPackagesFromProject(ctx, project, "Failed to sync packages after rollback"); err != nil {
			return err
		}

		if err := w.maybeReinstallEnv(ctx, project, wasInstalled, logWriter); err != nil {
			return err
		}

		fmt.Fprintf(logWriter, "Rollback completed successfully\n")

	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}

	return nil
}

func (w *Worker) createVersionSnapshot(ctx context.Context, project *models.Project, jobID uuid.UUID, userID uuid.UUID, description string) error {
	err := w.svc.CreateVersionSnapshot(ctx, project, jobID, userID, description)
	if err == nil {
		return nil
	}
	w.logger.Error("Failed to create version snapshot", "project_id", project.ID, "job_id", jobID, "error", err)

	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		return err
	}
	if isFatalResourceFailure(ctx, err) {
		return err
	}
	return nil
}

func (w *Worker) syncPackagesFromProject(ctx context.Context, project *models.Project, logMessage string) error {
	err := w.svc.SyncPackagesFromProject(ctx, project)
	if err == nil {
		return nil
	}
	w.logger.Error(logMessage, "error", err)

	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		return err
	}
	if isFatalResourceFailure(ctx, err) {
		return err
	}
	return nil
}

// maybeReinstallEnv reinstalls the environment after a lockfile-changing
// operation, but only in local mode and only when the project had an
// installed environment before the operation. This keeps installed
// environments in sync with the latest lockfile without ever implicitly
// installing a project the user never installed.
//
// By the time this runs, the manifest, lockfile, and version snapshot for
// the triggering operation are already committed, so ordinary reinstall
// failures are logged and recorded as install_status = install_failed instead
// of hiding a change that actually succeeded. Fatal resource/deadline failures
// still fail the job so processJob can run cleanup.
func (w *Worker) maybeReinstallEnv(ctx context.Context, project *models.Project, wasInstalled bool, logWriter io.Writer) error {
	if !w.svc.IsLocal() || !wasInstalled {
		return nil
	}
	fmt.Fprintf(logWriter, "Project was installed; reinstalling environment from updated lockfile...\n")
	if err := w.executor.InstallEnvironment(ctx, project, logWriter); err != nil {
		fmt.Fprintf(logWriter, "Reinstall failed, environment may be out of sync: %v\n", err)
		if recordErr := w.jobSvc.RecordFailedEnvInstall(project.ID, err.Error()); recordErr != nil {
			w.logger.Error("failed to record failed env install", "project_id", project.ID, "error", recordErr)
		}
		if isFatalResourceFailure(ctx, err) {
			return &autoReinstallFailureError{err: err}
		}
		return nil
	}
	w.svc.UpdateProjectSize(project)
	return nil
}

func isFatalResourceFailure(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return executor.IsResourceLimitError(err) || process.IsResourceLimitError(err)
}

// executeRollback restores project to a previous version
func (w *Worker) executeRollback(ctx context.Context, project *models.Project, version *models.ProjectVersion, logWriter io.Writer) error {
	envPath := w.svc.GetProjectPath(project)

	if err := w.svc.ValidateVersionContent(version.ManifestContent, version.LockFileContent); err != nil {
		return err
	}

	// 1. Write pixi.toml
	fmt.Fprintf(logWriter, "Restoring pixi.toml...\n")
	if err := writeFile(envPath, "pixi.toml", version.ManifestContent); err != nil {
		return err
	}

	// 2. Write pixi.lock
	fmt.Fprintf(logWriter, "Restoring pixi.lock...\n")
	if err := writeFile(envPath, "pixi.lock", version.LockFileContent); err != nil {
		return err
	}

	// 3. Refresh the lockfile against the restored manifest. The restored
	// pixi.lock is normally already consistent, so this is a fast no-op
	// that doubles as validation. Packages are not installed here.
	if err := w.executor.SolveEnvironment(ctx, project, logWriter); err != nil {
		return err
	}

	fmt.Fprintf(logWriter, "Project restored successfully\n")
	return nil
}

// buildCreateProjectOptions converts JobTypeCreate metadata into the
// executor's CreateProjectOptions. It is lenient — missing keys or
// non-string values yield zero-value fields rather than errors.
func buildCreateProjectOptions(metadata map[string]interface{}) executor.CreateProjectOptions {
	opts := executor.CreateProjectOptions{}
	if v, ok := metadata["pixi_toml"].(string); ok {
		opts.PixiToml = v
	}
	if v, ok := metadata["import_staging_dir"].(string); ok {
		opts.SeedDir = v
	}
	return opts
}

// parsePackagesFromMetadata extracts the packages list from job metadata,
// handling both []string and []interface{} (from JSON unmarshaling).
func parsePackagesFromMetadata(metadata map[string]any) []string {
	packagesInterface, ok := metadata["packages"]
	if !ok {
		return nil
	}

	switch v := packagesInterface.(type) {
	case []string:
		return v
	case []interface{}:
		packages := make([]string, len(v))
		for i, p := range v {
			packages[i] = fmt.Sprint(p)
		}
		return packages
	default:
		return nil
	}
}

// writeFile writes content to a file at the given base path.
func writeFile(basePath, filename, content string) error {
	return os.WriteFile(filepath.Join(basePath, filename), []byte(content), 0644)
}
