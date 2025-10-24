package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/aktech/darb/internal/executor"
	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/queue"
	"gorm.io/gorm"
)

// Worker processes jobs from the queue
type Worker struct {
	db       *gorm.DB
	queue    queue.Queue
	executor executor.Executor
	logger   *slog.Logger
}

// New creates a new worker instance
func New(db *gorm.DB, q queue.Queue, exec executor.Executor, logger *slog.Logger) *Worker {
	return &Worker{
		db:       db,
		queue:    q,
		executor: exec,
		logger:   logger,
	}
}

// Start begins processing jobs from the queue
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Worker started")

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Worker shutting down")
			return ctx.Err()
		default:
			job, err := w.queue.Dequeue(ctx)
			if err != nil {
				w.logger.Error("Failed to dequeue job", "error", err)
				time.Sleep(time.Second) // Backoff
				continue
			}

			if job == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Process the job
			w.processJob(ctx, job)
		}
	}
}

func (w *Worker) processJob(ctx context.Context, job *models.Job) {
	w.logger.Info("Processing job", "job_id", job.ID, "type", job.Type)

	// Update job status to running
	job.Status = models.JobStatusRunning
	now := time.Now()
	job.StartedAt = &now
	w.db.Save(job)

	// Create log buffer
	var logBuf bytes.Buffer

	// Execute the job
	err := w.executeJob(ctx, job, &logBuf)

	// Update job status
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Logs = logBuf.String()

	if err != nil {
		w.logger.Error("Job failed", "job_id", job.ID, "error", err)
		job.Status = models.JobStatusFailed
		job.Error = err.Error()
	} else {
		w.logger.Info("Job completed", "job_id", job.ID)
		job.Status = models.JobStatusCompleted
	}

	w.db.Save(job)
}

func (w *Worker) executeJob(ctx context.Context, job *models.Job, logWriter *bytes.Buffer) error {
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

	// Run pixi list to get installed packages
	cmd := exec.CommandContext(ctx, "pixi", "list", "--json")
	cmd.Dir = envPath

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run pixi list: %w", err)
	}

	// Parse JSON output
	var packages []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	if err := json.Unmarshal(output, &packages); err != nil {
		return fmt.Errorf("failed to parse pixi list output: %w", err)
	}

	// Clear existing packages for this environment
	w.db.Where("environment_id = ?", env.ID).Delete(&models.Package{})

	// Insert new packages
	for _, pkg := range packages {
		dbPkg := models.Package{
			EnvironmentID: env.ID,
			Name:          pkg.Name,
			Version:       pkg.Version,
		}
		if err := w.db.Create(&dbPkg).Error; err != nil {
			w.logger.Error("Failed to save package", "package", pkg.Name, "error", err)
		}
	}

	w.logger.Info("Synced packages from environment", "environment_id", env.ID, "count", len(packages))
	return nil
}
