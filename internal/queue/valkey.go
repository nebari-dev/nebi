package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/valkey-io/valkey-go"
	"gorm.io/gorm"
)

// ValkeyQueue implements a distributed job queue using Valkey
// Valkey is used for job transport (job IDs only), DB is source of truth
type ValkeyQueue struct {
	client valkey.Client
	db     *gorm.DB
	key    string // Queue key: "nebi:jobs"
}

// NewValkeyQueue creates a new Valkey-backed queue
func NewValkeyQueue(addr string, db *gorm.DB) (*ValkeyQueue, error) {
	if db == nil {
		return nil, fmt.Errorf("database instance is required for Valkey queue")
	}

	// Create Valkey client with connection pool
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Valkey: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pingCmd := client.B().Ping().Build()
	if err := client.Do(ctx, pingCmd).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping Valkey: %w", err)
	}

	q := &ValkeyQueue{
		client: client,
		db:     db,
		key:    "nebi:jobs",
	}

	slog.Info("Initialized Valkey job queue",
		"address", addr,
		"queue_key", q.key)
	return q, nil
}

// Enqueue adds a job to the queue
// 1. Save job to DB (source of truth)
// 2. Push job ID to Valkey list
func (q *ValkeyQueue) Enqueue(ctx context.Context, job *models.Job) error {
	if job.ID == uuid.Nil {
		return fmt.Errorf("job must have an ID")
	}

	// Save job to database first
	if err := q.db.WithContext(ctx).Save(job).Error; err != nil {
		return fmt.Errorf("failed to save job to database: %w", err)
	}

	// Marshal job ID to push to Valkey
	jobData, err := json.Marshal(map[string]string{
		"id":   job.ID.String(),
		"type": string(job.Type),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal job data: %w", err)
	}

	// Push to Valkey list (RPUSH for FIFO)
	cmd := q.client.B().Rpush().Key(q.key).Element(string(jobData)).Build()
	if err := q.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("failed to push job to Valkey: %w", err)
	}

	slog.Debug("Job enqueued",
		"job_id", job.ID,
		"type", job.Type,
		"queue_key", q.key)
	return nil
}

// Dequeue retrieves the next job from the queue (blocking)
// 1. BLPOP from Valkey (blocking pop with timeout)
// 2. Parse job ID
// 3. Fetch full job from DB
func (q *ValkeyQueue) Dequeue(ctx context.Context) (*models.Job, error) {
	// BLPOP with 5 second timeout
	cmd := q.client.B().Blpop().Key(q.key).Timeout(5).Build()
	result := q.client.Do(ctx, cmd)

	// Parse BLPOP result [key, value]
	values, err := result.AsStrSlice()
	if err != nil {
		// BLPOP timeout or no jobs available - treat as normal timeout
		// AsStrSlice returns an error when BLPOP times out (valkey nil message)
		// This is expected behavior when the queue is empty
		return nil, context.DeadlineExceeded
	}
	if len(values) < 2 {
		return nil, fmt.Errorf("invalid BLPOP result: expected 2 values, got %d", len(values))
	}

	// Parse job data (second element contains the job info)
	var jobData map[string]string
	if err := json.Unmarshal([]byte(values[1]), &jobData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job data: %w", err)
	}

	jobID, err := uuid.Parse(jobData["id"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse job ID: %w", err)
	}

	// Fetch full job from database
	var job models.Job
	if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch job from database: %w", err)
	}

	slog.Debug("Job dequeued",
		"job_id", job.ID,
		"type", job.Type)
	return &job, nil
}

// GetStatus retrieves the current status of a job from the database
func (q *ValkeyQueue) GetStatus(ctx context.Context, jobID uuid.UUID) (*models.Job, error) {
	var job models.Job
	if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("failed to get job status: %w", err)
	}
	return &job, nil
}

// UpdateStatus updates the status of a job in the database
func (q *ValkeyQueue) UpdateStatus(ctx context.Context, jobID uuid.UUID, status models.JobStatus, logs string) error {
	updates := map[string]interface{}{
		"status": status,
	}

	// Append logs if provided
	if logs != "" {
		var job models.Job
		if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
			return fmt.Errorf("failed to fetch job: %w", err)
		}
		if job.Logs != "" {
			updates["logs"] = job.Logs + "\n" + logs
		} else {
			updates["logs"] = logs
		}
	}

	result := q.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", jobID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update job status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrJobNotFound
	}

	slog.Debug("Job status updated",
		"job_id", jobID,
		"status", status)
	return nil
}

// Complete marks a job as completed in the database
func (q *ValkeyQueue) Complete(ctx context.Context, jobID uuid.UUID, logs string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":       models.JobStatusCompleted,
		"completed_at": now,
	}

	// Append logs if provided
	if logs != "" {
		var job models.Job
		if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
			return fmt.Errorf("failed to fetch job: %w", err)
		}
		if job.Logs != "" {
			updates["logs"] = job.Logs + "\n" + logs
		} else {
			updates["logs"] = logs
		}
	}

	result := q.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", jobID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to complete job: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrJobNotFound
	}

	slog.Info("Job completed", "job_id", jobID)
	return nil
}

// Fail marks a job as failed in the database
func (q *ValkeyQueue) Fail(ctx context.Context, jobID uuid.UUID, errorMsg string, logs string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":       models.JobStatusFailed,
		"error":        errorMsg,
		"completed_at": now,
	}

	// Append logs if provided
	if logs != "" {
		var job models.Job
		if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
			return fmt.Errorf("failed to fetch job: %w", err)
		}
		if job.Logs != "" {
			updates["logs"] = job.Logs + "\n" + logs
		} else {
			updates["logs"] = logs
		}
	}

	result := q.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", jobID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to mark job as failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrJobNotFound
	}

	slog.Error("Job failed",
		"job_id", jobID,
		"error", errorMsg)
	return nil
}

// GetClient returns the underlying Valkey client
// Used for distributed log streaming via pub/sub
func (q *ValkeyQueue) GetClient() valkey.Client {
	return q.client
}

// Close closes the Valkey connection
func (q *ValkeyQueue) Close() error {
	q.client.Close()
	slog.Info("Valkey queue closed")
	return nil
}
