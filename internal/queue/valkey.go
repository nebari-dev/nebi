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

const enqueueTenantJobScript = `
local was_empty = redis.call("LLEN", KEYS[1]) == 0
redis.call("RPUSH", KEYS[1], ARGV[1])
if was_empty then
  local score = redis.call("INCR", KEYS[3])
  redis.call("ZADD", KEYS[2], score, ARGV[2])
end
return 1
`

const popTenantJobScript = `
local tenant = redis.call("ZPOPMIN", KEYS[1], 1)
if #tenant == 0 then
  return {}
end
local tenant_name = tenant[1]
local queue_key = ARGV[1] .. tenant_name
local job = redis.call("LPOP", queue_key)
if not job then
  return {tenant_name, ""}
end
if redis.call("LLEN", queue_key) > 0 then
  local score = redis.call("INCR", KEYS[2])
  redis.call("ZADD", KEYS[1], score, tenant_name)
end
return {tenant_name, job}
`

// ValkeyQueue implements a distributed job queue using Valkey
// Valkey is used for job transport (job IDs only), DB is source of truth
type ValkeyQueue struct {
	client     valkey.Client
	db         *gorm.DB
	key        string // Queue key prefix: "nebi:jobs"
	tenantsKey string
	seqKey     string
}

// NewValkeyQueue creates a new Valkey-backed queue
func NewValkeyQueue(addr string, db *gorm.DB) (*ValkeyQueue, error) {
	if db == nil {
		return nil, fmt.Errorf("database instance is required for Valkey queue")
	}

	// Create Valkey client with connection pool
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{addr},
		DisableCache: true,
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
		client:     client,
		db:         db,
		key:        "nebi:{jobs}",
		tenantsKey: "nebi:{jobs}:tenants",
		seqKey:     "nebi:{jobs}:tenant-seq",
	}

	slog.Info("Initialized Valkey job queue",
		"address", addr,
		"queue_key", q.key)
	return q, nil
}

// Enqueue adds a job to the queue
// 1. Save job to DB (source of truth)
// 2. Push job ID to a per-tenant Valkey list
// 3. Register an idle tenant once on the round-robin tenant set
func (q *ValkeyQueue) Enqueue(ctx context.Context, job *models.Job) error {
	if job.ID == uuid.Nil {
		return fmt.Errorf("job must have an ID")
	}

	// Save job to database first
	if err := q.db.WithContext(ctx).Save(job).Error; err != nil {
		return fmt.Errorf("failed to save job to database: %w", err)
	}

	tenant := tenantKeyForJob(job)
	// Marshal job ID to push to Valkey
	jobData, err := json.Marshal(map[string]string{
		"id": job.ID.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal job data: %w", err)
	}

	cmd := q.client.B().Eval().
		Script(enqueueTenantJobScript).
		Numkeys(3).
		Key(q.tenantQueueKey(tenant), q.tenantsKey, q.seqKey).
		Arg(string(jobData), tenant).
		Build()
	if err := q.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("failed to push job to Valkey: %w", err)
	}

	slog.Debug("Job enqueued",
		"job_id", job.ID,
		"type", job.Type,
		"tenant", tenant,
		"queue_key", q.tenantQueueKey(tenant))
	return nil
}

// Dequeue retrieves the next job from the queue (blocking)
// 1. Atomically pop the next tenant and one job from that tenant
// 2. Parse job ID
// 3. Fetch full job from DB
func (q *ValkeyQueue) Dequeue(ctx context.Context) (*models.Job, error) {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		tenant, jobData, err := q.popTenantJob(ctx)
		if err != nil {
			return nil, err
		}
		if jobData == "" {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-deadline.C:
				return nil, context.DeadlineExceeded
			case <-ticker.C:
				continue
			}
		}

		job, err := q.loadDequeuedJob(ctx, jobData)
		if err != nil {
			return nil, err
		}
		slog.Debug("Job dequeued", "job_id", job.ID, "type", job.Type, "tenant", tenant)
		return job, nil
	}
}

func (q *ValkeyQueue) tenantQueueKey(tenant string) string {
	return q.key + ":tenant:" + tenant
}

func (q *ValkeyQueue) popTenantJob(ctx context.Context) (string, string, error) {
	cmd := q.client.B().Eval().
		Script(popTenantJobScript).
		Numkeys(2).
		Key(q.tenantsKey, q.seqKey).
		Arg(q.key + ":tenant:").
		Build()
	values, err := q.client.Do(ctx, cmd).AsStrSlice()
	if err != nil {
		return "", "", fmt.Errorf("failed to pop job from Valkey: %w", err)
	}
	if len(values) == 0 {
		return "", "", nil
	}
	if len(values) != 2 {
		return "", "", fmt.Errorf("invalid pop result: expected tenant and job, got %d values", len(values))
	}
	return values[0], values[1], nil
}

func (q *ValkeyQueue) loadDequeuedJob(ctx context.Context, encoded string) (*models.Job, error) {
	var jobData map[string]string
	if err := json.Unmarshal([]byte(encoded), &jobData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job data: %w", err)
	}

	jobID, err := uuid.Parse(jobData["id"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse job ID: %w", err)
	}

	var job models.Job
	if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch job from database: %w", err)
	}
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
