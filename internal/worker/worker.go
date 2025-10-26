package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/aktech/darb/internal/executor"
	"github.com/aktech/darb/internal/logstream"
	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/pkgmgr"
	_ "github.com/aktech/darb/internal/pkgmgr/pixi" // Register pixi
	_ "github.com/aktech/darb/internal/pkgmgr/uv"   // Register uv
	"github.com/aktech/darb/internal/queue"
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

	// Create log buffer
	var logBuf bytes.Buffer

	// Create broker writer for in-memory streaming
	brokerWriter := logstream.NewStreamWriter(job.ID, w.broker, &logBuf)

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

	// Close broker subscriptions for this job
	defer w.broker.Close(job.ID)

	// Update job status
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Logs = logBuf.String()

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

func (w *Worker) executeJob(ctx context.Context, job *models.Job, logWriter io.Writer) error {
	// Load environment
	var env models.Environment
	if err := w.db.First(&env, job.EnvironmentID).Error; err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	switch job.Type {
	case models.JobTypeCreate:
		env.Status = models.EnvStatusCreating
		w.db.Save(&env)

		// Check if pixi_toml content is provided in metadata
		var pixiToml string
		if pixiTomlInterface, ok := job.Metadata["pixi_toml"]; ok {
			pixiToml, _ = pixiTomlInterface.(string)
		}

		if err := w.executor.CreateEnvironment(ctx, &env, logWriter, pixiToml); err != nil {
			env.Status = models.EnvStatusFailed
			w.db.Save(&env)
			return err
		}

		// List installed packages and save to database
		if err := w.syncPackagesFromEnvironment(ctx, &env); err != nil {
			w.logger.Error("Failed to sync packages", "error", err)
		}

		env.Status = models.EnvStatusReady
		w.db.Save(&env)

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

		if err := w.executor.InstallPackages(ctx, &env, packages, logWriter); err != nil {
			return err
		}

		// Store installed packages in database
		for _, pkgName := range packages {
			pkg := models.Package{
				EnvironmentID: env.ID,
				Name:          pkgName,
				Version:       "", // TODO: Extract version from pixi
			}
			w.db.Create(&pkg)
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

		if err := w.executor.RemovePackages(ctx, &env, packages, logWriter); err != nil {
			return err
		}

		// Remove packages from database
		for _, pkgName := range packages {
			w.db.Where("environment_id = ? AND name = ?", env.ID, pkgName).Delete(&models.Package{})
		}

	case models.JobTypeDelete:
		env.Status = models.EnvStatusDeleting
		w.db.Save(&env)

		if err := w.executor.DeleteEnvironment(ctx, &env, logWriter); err != nil {
			return err
		}

		// Delete all packages first
		w.db.Where("environment_id = ?", env.ID).Delete(&models.Package{})

		// Soft delete the environment
		w.db.Delete(&env)

	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}

	return nil
}

// syncPackagesFromEnvironment lists packages from the environment and saves them to the database
func (w *Worker) syncPackagesFromEnvironment(ctx context.Context, env *models.Environment) error {
	envPath := w.executor.GetEnvironmentPath(env)

	// Create package manager for this environment
	pm, err := pkgmgr.New(env.PackageManager)
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
	w.db.Where("environment_id = ?", env.ID).Delete(&models.Package{})

	// Insert new packages
	for _, pkg := range pkgs {
		dbPkg := models.Package{
			EnvironmentID: env.ID,
			Name:          pkg.Name,
			Version:       pkg.Version,
		}
		if err := w.db.Create(&dbPkg).Error; err != nil {
			w.logger.Error("Failed to save package", "package", pkg.Name, "error", err)
		}
	}

	w.logger.Info("Synced packages from environment", "environment_id", env.ID, "count", len(pkgs))
	return nil
}
