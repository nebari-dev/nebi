package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/valkey-io/valkey-go"
)

// Worker processes jobs from the queue
type Worker struct {
	queue        queue.Queue
	executor     executor.Executor
	svc          *service.WorkspaceService
	jobSvc       *service.JobService
	logger       *slog.Logger
	broker       *logstream.LogBroker
	valkeyClient valkey.Client // For distributed log streaming (optional, can be nil for local mode)
	maxWorkers   int
	semaphore    chan struct{}
	wg           sync.WaitGroup
}

// New creates a new worker instance
func New(q queue.Queue, exec executor.Executor, svc *service.WorkspaceService, jobSvc *service.JobService, logger *slog.Logger, valkeyClient valkey.Client) *Worker {
	maxWorkers := 10 // Allow up to 10 concurrent jobs
	return &Worker{
		queue:        q,
		executor:     exec,
		svc:          svc,
		jobSvc:       jobSvc,
		logger:       logger,
		broker:       logstream.NewBroker(),
		valkeyClient: valkeyClient,
		maxWorkers:   maxWorkers,
		semaphore:    make(chan struct{}, maxWorkers),
	}
}

// GetBroker returns the log broker for external access (SSE endpoints)
func (w *Worker) GetBroker() *logstream.LogBroker {
	return w.broker
}

// Start begins processing jobs from the queue
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Worker started", "max_concurrent_jobs", w.maxWorkers)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Worker shutting down, waiting for jobs to complete")
			w.wg.Wait() // Wait for all jobs to complete
			w.logger.Info("All jobs completed, worker stopped")
			return ctx.Err()
		default:
			job, err := w.queue.Dequeue(ctx)
			if err != nil {
				// DeadlineExceeded means no jobs available (normal timeout), not an error
				if err == context.DeadlineExceeded {
					// No jobs available, just continue polling
					continue
				}
				// Actual errors (connection issues, etc.)
				w.logger.Error("Failed to dequeue job", "error", err)
				time.Sleep(time.Second) // Backoff on real errors
				continue
			}

			if job == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Acquire semaphore slot (blocks if max workers reached)
			select {
			case w.semaphore <- struct{}{}:
				// Got a slot, process job asynchronously
				w.wg.Add(1)
				go func(j *models.Job) {
					defer w.wg.Done()
					defer func() { <-w.semaphore }() // Release slot when done

					w.processJob(ctx, j)
				}(job)
			case <-ctx.Done():
				w.logger.Info("Context cancelled while waiting for worker slot")
				return ctx.Err()
			}
		}
	}
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

	// Create thread-safe log buffer
	var logBuf bytes.Buffer
	var logMutex sync.Mutex

	// Start periodic log persistence (flush to DB every 2 seconds)
	stopFlushing := make(chan struct{})
	defer func() {
		close(stopFlushing)
	}()

	go w.flushLogsToDatabase(job.ID, &logBuf, &logMutex, stopFlushing)

	// Close broker subscriptions when job finishes
	defer w.broker.Close(job.ID)

	// Thread-safe writer wrapper
	safeWriter := &threadSafeWriter{writer: &logBuf, mu: &logMutex}

	// Create broker writer for in-memory streaming
	brokerWriter := logstream.NewStreamWriter(job.ID, w.broker, safeWriter)

	// Create multi-writer: buffer + broker (in-memory) + Valkey (distributed, if available)
	var logWriter io.Writer
	if w.valkeyClient != nil {
		// Create Valkey log writer for distributed streaming
		valkeyWriter := logstream.NewValkeyLogWriter(w.valkeyClient, job.ID.String())
		logWriter = io.MultiWriter(brokerWriter, valkeyWriter)
	} else {
		// Use only in-memory broker for local mode
		logWriter = brokerWriter
	}

	// Execute the job with streaming logs
	err := w.executeJob(ctx, job, logWriter)

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
		if w.valkeyClient != nil {
			valkeyWriter := logstream.NewValkeyLogWriter(w.valkeyClient, job.ID.String())
			valkeyWriter.Publish(errorMsg)
		}
	} else {
		w.logger.Info("Job completed", "job_id", job.ID)
		w.jobSvc.MarkCompleted(job, finalLogs)
		// Publish completion to subscribers
		completionMsg := "\n[COMPLETED] Job finished successfully\n"
		w.broker.Publish(job.ID, completionMsg)
		if w.valkeyClient != nil {
			valkeyWriter := logstream.NewValkeyLogWriter(w.valkeyClient, job.ID.String())
			valkeyWriter.Publish(completionMsg)
			valkeyWriter.SetTTL(3600)
		}
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

func (w *Worker) executeJob(ctx context.Context, job *models.Job, logWriter io.Writer) error {
	// Load workspace
	ws, err := w.jobSvc.LoadWorkspace(job.WorkspaceID)
	if err != nil {
		return err
	}

	// Extract user ID from job metadata (if present)
	userID := ws.OwnerID
	if userIDInterface, ok := job.Metadata["user_id"]; ok {
		if userIDStr, ok := userIDInterface.(string); ok {
			if parsed, err := uuid.Parse(userIDStr); err == nil {
				userID = parsed
			}
		}
	}

	switch job.Type {
	case models.JobTypeCreate:
		w.svc.SetWorkspaceStatus(ws.ID, models.WsStatusCreating)

		opts := buildCreateWorkspaceOptions(job.Metadata)

		if err := w.executor.CreateWorkspace(ctx, ws, logWriter, opts); err != nil {
			w.svc.SetWorkspaceStatus(ws.ID, models.WsStatusFailed)
			return err
		}

		// Persist the resolved path so the CLI can find the workspace on disk
		if ws.Path == "" {
			w.svc.SetWorkspacePath(ws.ID, w.executor.GetWorkspacePath(ws))
		}

		// List installed packages and save to database
		if err := w.svc.SyncPackagesFromWorkspace(ctx, ws); err != nil {
			w.logger.Error("Failed to sync packages", "error", err)
		}

		w.svc.UpdateWorkspaceSize(ws)
		w.svc.SetWorkspaceStatus(ws.ID, models.WsStatusReady)

		// Create version snapshot
		if err := w.svc.CreateVersionSnapshot(ctx, ws, job.ID, userID, "Initial workspace creation"); err != nil {
			w.logger.Error("Failed to create version snapshot", "error", err)
		}

	case models.JobTypeInstall:
		packages := parsePackagesFromMetadata(job.Metadata)
		if packages == nil {
			return fmt.Errorf("packages not found in job metadata")
		}

		if err := w.executor.InstallPackages(ctx, ws, packages, logWriter); err != nil {
			return err
		}

		w.svc.SaveInstalledPackages(ws.ID, packages)
		w.svc.UpdateWorkspaceSize(ws)

		if err := w.svc.CreateVersionSnapshot(ctx, ws, job.ID, userID, fmt.Sprintf("Installed packages: %v", packages)); err != nil {
			w.logger.Error("Failed to create version snapshot", "error", err)
		}

	case models.JobTypeRemove:
		packages := parsePackagesFromMetadata(job.Metadata)
		if packages == nil {
			return fmt.Errorf("packages not found in job metadata")
		}

		if err := w.executor.RemovePackages(ctx, ws, packages, logWriter); err != nil {
			return err
		}

		w.svc.DeletePackagesByName(ws.ID, packages)
		w.svc.UpdateWorkspaceSize(ws)

		if err := w.svc.CreateVersionSnapshot(ctx, ws, job.ID, userID, fmt.Sprintf("Removed packages: %v", packages)); err != nil {
			w.logger.Error("Failed to create version snapshot", "error", err)
		}

	case models.JobTypeUpdate:
		fmt.Fprintf(logWriter, "Solving environment from current pixi.toml...\n")

		if err := w.executor.SolveEnvironment(ctx, ws, logWriter); err != nil {
			return err
		}

		if err := w.svc.SyncPackagesFromWorkspace(ctx, ws); err != nil {
			w.logger.Error("Failed to sync packages after solve", "error", err)
		}

		w.svc.UpdateWorkspaceSize(ws)

		if err := w.svc.CreateVersionSnapshot(ctx, ws, job.ID, userID, "Solved environment from updated pixi.toml"); err != nil {
			w.logger.Error("Failed to create version snapshot", "error", err)
		}

	case models.JobTypeDelete:
		w.svc.SetWorkspaceStatus(ws.ID, models.WsStatusDeleting)

		if err := w.executor.DeleteWorkspace(ctx, ws, logWriter); err != nil {
			return err
		}

		w.svc.DeleteAllPackages(ws.ID)
		w.svc.SoftDeleteWorkspace(ws.ID)

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

		if version.WorkspaceID != ws.ID {
			return fmt.Errorf("version does not belong to this workspace")
		}

		fmt.Fprintf(logWriter, "Rolling back to version %d\n", version.VersionNumber)

		if err := w.executeRollback(ctx, ws, version, logWriter); err != nil {
			return err
		}

		if err := w.svc.SyncPackagesFromWorkspace(ctx, ws); err != nil {
			w.logger.Error("Failed to sync packages after rollback", "error", err)
		}

		w.svc.UpdateWorkspaceSize(ws)

		if err := w.svc.CreateVersionSnapshot(ctx, ws, job.ID, userID, fmt.Sprintf("Rolled back to version %d", version.VersionNumber)); err != nil {
			w.logger.Error("Failed to create version snapshot after rollback", "error", err)
		}

		fmt.Fprintf(logWriter, "Rollback completed successfully\n")

	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}

	return nil
}

// executeRollback restores workspace to a previous version
func (w *Worker) executeRollback(ctx context.Context, ws *models.Workspace, version *models.WorkspaceVersion, logWriter io.Writer) error {
	envPath := w.svc.GetWorkspacePath(ws)

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

	// 3. Run pixi install to recreate environment
	fmt.Fprintf(logWriter, "Running pixi install to apply changes...\n")

	cmd := exec.CommandContext(ctx, "pixi", "install", "-v")
	cmd.Dir = envPath
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pixi install failed: %w", err)
	}

	fmt.Fprintf(logWriter, "Workspace restored successfully\n")
	return nil
}

// buildCreateWorkspaceOptions converts JobTypeCreate metadata into the
// executor's CreateWorkspaceOptions. It is lenient — missing keys or
// non-string values yield zero-value fields rather than errors.
func buildCreateWorkspaceOptions(metadata map[string]interface{}) executor.CreateWorkspaceOptions {
	opts := executor.CreateWorkspaceOptions{}
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
