package worker

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	_ "github.com/nebari-dev/nebi/internal/pkgmgr/pixi" // Register pixi
	_ "github.com/nebari-dev/nebi/internal/pkgmgr/uv"   // Register uv
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/utils"
	"github.com/valkey-io/valkey-go"
	"gorm.io/gorm"
)

// Worker processes jobs from the queue
type Worker struct {
	db           *gorm.DB
	queue        queue.Queue
	executor     executor.Executor
	logger       *slog.Logger
	broker       *logstream.LogBroker
	valkeyClient valkey.Client // For distributed log streaming (optional, can be nil for local mode)
	maxWorkers   int
	semaphore    chan struct{}
	wg           sync.WaitGroup
}

// New creates a new worker instance
func New(db *gorm.DB, q queue.Queue, exec executor.Executor, logger *slog.Logger, valkeyClient valkey.Client) *Worker {
	maxWorkers := 10 // Allow up to 10 concurrent jobs
	return &Worker{
		db:           db,
		queue:        q,
		executor:     exec,
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
			// Update job as failed
			completedAt := time.Now()
			job.CompletedAt = &completedAt
			job.Status = models.JobStatusFailed
			job.Error = fmt.Sprintf("Job panicked: %v", r)
			w.db.Save(job)
		}
	}()

	w.logger.Info("Processing job", "job_id", job.ID, "type", job.Type)

	// Update job status to running
	job.Status = models.JobStatusRunning
	now := time.Now()
	job.StartedAt = &now
	w.db.Save(job)

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
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Logs = finalLogs

	if err != nil {
		w.logger.Error("Job failed", "job_id", job.ID, "error", err)
		job.Status = models.JobStatusFailed
		job.Error = err.Error()
		// Publish error to subscribers
		errorMsg := fmt.Sprintf("\n[ERROR] Job failed: %v\n", err)
		w.broker.Publish(job.ID, errorMsg)
		// Also publish to Valkey if available
		if w.valkeyClient != nil {
			valkeyWriter := logstream.NewValkeyLogWriter(w.valkeyClient, job.ID.String())
			valkeyWriter.Publish(errorMsg)
		}
	} else {
		w.logger.Info("Job completed", "job_id", job.ID)
		job.Status = models.JobStatusCompleted
		// Publish completion to subscribers
		completionMsg := "\n[COMPLETED] Job finished successfully\n"
		w.broker.Publish(job.ID, completionMsg)
		// Also publish to Valkey if available
		if w.valkeyClient != nil {
			valkeyWriter := logstream.NewValkeyLogWriter(w.valkeyClient, job.ID.String())
			valkeyWriter.Publish(completionMsg)
			// Set TTL on Valkey log channel for cleanup (1 hour)
			valkeyWriter.SetTTL(3600)
		}
	}

	w.db.Save(job)
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
			if err := w.db.Model(&models.Job{}).Where("id = ?", jobID).Update("logs", currentLogs).Error; err != nil {
				w.logger.Error("Failed to flush logs to database", "job_id", jobID, "error", err)
			}

		case <-stop:
			// Final flush before stopping
			logMutex.Lock()
			finalLogs := logBuf.String()
			logMutex.Unlock()

			if err := w.db.Model(&models.Job{}).Where("id = ?", jobID).Update("logs", finalLogs).Error; err != nil {
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
	// Load environment
	var ws models.Workspace
	if err := w.db.First(&ws, job.WorkspaceID).Error; err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}

	switch job.Type {
	case models.JobTypeCreate:
		ws.Status = models.WsStatusCreating
		w.db.Save(&ws)

		// Check if pixi_toml content is provided in metadata
		var pixiToml string
		if pixiTomlInterface, ok := job.Metadata["pixi_toml"]; ok {
			pixiToml, _ = pixiTomlInterface.(string)
		}

		if err := w.executor.CreateWorkspace(ctx, &ws, logWriter, pixiToml); err != nil {
			ws.Status = models.WsStatusFailed
			w.db.Save(&ws)
			return err
		}

		// Persist the resolved path so the CLI can find the workspace on disk
		if ws.Path == "" {
			ws.Path = w.executor.GetWorkspacePath(&ws)
		}

		// List installed packages and save to database
		if err := w.syncPackagesFromWorkspace(ctx, &ws); err != nil {
			w.logger.Error("Failed to sync packages", "error", err)
		}

		// Update workspace size
		w.updateWorkspaceSize(&ws)

		ws.Status = models.WsStatusReady
		w.db.Save(&ws)

		// Create version snapshot
		if err := w.createVersionSnapshot(ctx, &ws, job, "Initial workspace creation"); err != nil {
			w.logger.Error("Failed to create version snapshot", "error", err)
		}

	case models.JobTypeInstall:
		// Parse packages from job metadata
		packagesInterface, ok := job.Metadata["packages"]
		if !ok {
			return fmt.Errorf("packages not found in job metadata")
		}

		// Handle both []string and []interface{} (from JSON unmarshaling)
		var packages []string
		switch v := packagesInterface.(type) {
		case []string:
			packages = v
		case []interface{}:
			packages = make([]string, len(v))
			for i, p := range v {
				packages[i] = fmt.Sprint(p)
			}
		default:
			return fmt.Errorf("invalid packages type in job metadata: %T", packagesInterface)
		}

		if err := w.executor.InstallPackages(ctx, &ws, packages, logWriter); err != nil {
			return err
		}

		// Store installed packages in database
		for _, pkgName := range packages {
			pkg := models.Package{
				WorkspaceID: ws.ID,
				Name:        pkgName,
				Version:     "", // TODO: Extract version from pixi
				InstalledAt: time.Now(),
			}
			w.db.Create(&pkg)
		}

		// Update workspace size
		w.updateWorkspaceSize(&ws)
		w.db.Save(&ws)

		// Create version snapshot
		description := fmt.Sprintf("Installed packages: %v", packages)
		if err := w.createVersionSnapshot(ctx, &ws, job, description); err != nil {
			w.logger.Error("Failed to create version snapshot", "error", err)
		}

	case models.JobTypeRemove:
		// Parse packages from job metadata
		packagesInterface, ok := job.Metadata["packages"]
		if !ok {
			return fmt.Errorf("packages not found in job metadata")
		}

		var packages []string
		switch v := packagesInterface.(type) {
		case []string:
			packages = v
		case []interface{}:
			packages = make([]string, len(v))
			for i, p := range v {
				packages[i] = fmt.Sprint(p)
			}
		default:
			return fmt.Errorf("invalid packages type in job metadata: %T", packagesInterface)
		}

		if err := w.executor.RemovePackages(ctx, &ws, packages, logWriter); err != nil {
			return err
		}

		// Remove packages from database
		for _, pkgName := range packages {
			w.db.Where("workspace_id = ? AND name = ?", ws.ID, pkgName).Delete(&models.Package{})
		}

		// Update workspace size
		w.updateWorkspaceSize(&ws)
		w.db.Save(&ws)

		// Create version snapshot
		description := fmt.Sprintf("Removed packages: %v", packages)
		if err := w.createVersionSnapshot(ctx, &ws, job, description); err != nil {
			w.logger.Error("Failed to create version snapshot", "error", err)
		}

	case models.JobTypeDelete:
		ws.Status = models.WsStatusDeleting
		w.db.Save(&ws)

		if err := w.executor.DeleteWorkspace(ctx, &ws, logWriter); err != nil {
			return err
		}

		// Delete all packages first
		w.db.Where("workspace_id = ?", ws.ID).Delete(&models.Package{})

		// Soft delete the environment
		w.db.Delete(&ws)

	case models.JobTypeRollback:
		// Parse version ID from metadata
		versionIDStr, ok := job.Metadata["version_id"].(string)
		if !ok {
			return fmt.Errorf("version_id not found in job metadata")
		}

		versionID, err := uuid.Parse(versionIDStr)
		if err != nil {
			return fmt.Errorf("invalid version_id: %w", err)
		}

		// Fetch version
		var version models.WorkspaceVersion
		if err := w.db.First(&version, versionID).Error; err != nil {
			return fmt.Errorf("failed to load version: %w", err)
		}

		// Verify version belongs to this environment
		if version.WorkspaceID != ws.ID {
			return fmt.Errorf("version does not belong to this workspace")
		}

		fmt.Fprintf(logWriter, "Rolling back to version %d\n", version.VersionNumber)

		// Execute rollback
		if err := w.executeRollback(ctx, &ws, &version, logWriter); err != nil {
			return err
		}

		// Sync packages from environment
		if err := w.syncPackagesFromWorkspace(ctx, &ws); err != nil {
			w.logger.Error("Failed to sync packages after rollback", "error", err)
		}

		// Update workspace size
		w.updateWorkspaceSize(&ws)
		w.db.Save(&ws)

		// Create new version snapshot for the rollback
		description := fmt.Sprintf("Rolled back to version %d", version.VersionNumber)
		if err := w.createVersionSnapshot(ctx, &ws, job, description); err != nil {
			w.logger.Error("Failed to create version snapshot after rollback", "error", err)
		}

		fmt.Fprintf(logWriter, "Rollback completed successfully\n")

	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}

	return nil
}

// executeRollback restores environment to a previous version
func (w *Worker) executeRollback(ctx context.Context, ws *models.Workspace, version *models.WorkspaceVersion, logWriter io.Writer) error {
	envPath := w.executor.GetWorkspacePath(ws)

	// 1. Write pixi.toml
	manifestPath := filepath.Join(envPath, "pixi.toml")
	fmt.Fprintf(logWriter, "Restoring pixi.toml...\n")
	if err := os.WriteFile(manifestPath, []byte(version.ManifestContent), 0644); err != nil {
		return fmt.Errorf("failed to write pixi.toml: %w", err)
	}

	// 2. Write pixi.lock
	lockPath := filepath.Join(envPath, "pixi.lock")
	fmt.Fprintf(logWriter, "Restoring pixi.lock...\n")
	if err := os.WriteFile(lockPath, []byte(version.LockFileContent), 0644); err != nil {
		return fmt.Errorf("failed to write pixi.lock: %w", err)
	}

	// 3. Run pixi install to recreate environment
	fmt.Fprintf(logWriter, "Running pixi install to apply changes...\n")

	// Use pixi binary from PATH
	pixiBinary := "pixi"

	// Run pixi install
	cmd := exec.CommandContext(ctx, pixiBinary, "install", "-v")
	cmd.Dir = envPath
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pixi install failed: %w", err)
	}

	fmt.Fprintf(logWriter, "Workspace restored successfully\n")
	return nil
}

// syncPackagesFromWorkspace lists packages from the environment and saves them to the database
func (w *Worker) syncPackagesFromWorkspace(ctx context.Context, ws *models.Workspace) error {
	envPath := w.executor.GetWorkspacePath(ws)

	// Create package manager for this environment
	pm, err := pkgmgr.New(ws.PackageManager)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	// List packages using the package manager
	pkgs, err := pm.List(ctx, pkgmgr.ListOptions{
		EnvPath: envPath,
	})
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}

	// Clear existing packages for this environment
	w.db.Where("workspace_id = ?", ws.ID).Delete(&models.Package{})

	// Insert new packages
	for _, pkg := range pkgs {
		dbPkg := models.Package{
			WorkspaceID: ws.ID,
			Name:        pkg.Name,
			Version:     pkg.Version,
		}
		if err := w.db.Create(&dbPkg).Error; err != nil {
			w.logger.Error("Failed to save package", "package", pkg.Name, "error", err)
		}
	}

	w.logger.Info("Synced packages from workspace", "workspace_id", ws.ID, "count", len(pkgs))
	return nil
}

// updateWorkspaceSize calculates and updates the workspace size in the database
func (w *Worker) updateWorkspaceSize(ws *models.Workspace) {
	envPath := w.executor.GetWorkspacePath(ws)
	sizeBytes, err := utils.GetDirectorySize(envPath)
	if err != nil {
		w.logger.Warn("Failed to calculate workspace size", "ws_id", ws.ID, "error", err)
		return
	}

	ws.SizeBytes = sizeBytes
	w.logger.Info("Updated workspace size", "ws_id", ws.ID, "size", utils.FormatBytes(sizeBytes))
}

// createVersionSnapshot creates a version snapshot after a successful operation
func (w *Worker) createVersionSnapshot(ctx context.Context, ws *models.Workspace, job *models.Job, description string) error {
	envPath := w.executor.GetWorkspacePath(ws)

	// Read pixi.toml
	manifestPath := filepath.Join(envPath, "pixi.toml")
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read pixi.toml: %w", err)
	}

	// Read pixi.lock
	lockPath := filepath.Join(envPath, "pixi.lock")
	lockContent, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("failed to read pixi.lock: %w", err)
	}

	// Get package list from package manager
	pm, err := pkgmgr.New(ws.PackageManager)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	pkgs, err := pm.List(ctx, pkgmgr.ListOptions{EnvPath: envPath})
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}

	// Serialize package list to JSON
	packageMetadata, err := json.Marshal(pkgs)
	if err != nil {
		return fmt.Errorf("failed to serialize package metadata: %w", err)
	}

	// Get user ID from job metadata or environment owner
	var createdBy uuid.UUID
	if userIDInterface, ok := job.Metadata["user_id"]; ok {
		if userIDStr, ok := userIDInterface.(string); ok {
			createdBy, _ = uuid.Parse(userIDStr)
		}
	}
	if createdBy == uuid.Nil {
		createdBy = ws.OwnerID
	}

	// Create version record
	version := models.WorkspaceVersion{
		WorkspaceID:     ws.ID,
		LockFileContent: string(lockContent),
		ManifestContent: string(manifestContent),
		PackageMetadata: string(packageMetadata),
		JobID:           &job.ID,
		CreatedBy:       createdBy,
		Description:     description,
	}

	if err := w.db.Create(&version).Error; err != nil {
		return fmt.Errorf("failed to create version snapshot: %w", err)
	}

	w.logger.Info("Created version snapshot",
		"workspace_id", ws.ID,
		"version_number", version.VersionNumber,
		"job_id", job.ID)

	return nil
}
