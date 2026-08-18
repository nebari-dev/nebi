package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/valkey-io/valkey-go"
	"gorm.io/gorm"
)

const enqueueTenantJobScript = `
local added = redis.call("SADD", KEYS[4], ARGV[1])
if added == 0 then
  return 0
end
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
redis.call("SREM", KEYS[3], job)
if redis.call("LLEN", queue_key) > 0 then
  local score = redis.call("INCR", KEYS[2])
  redis.call("ZADD", KEYS[1], score, tenant_name)
end
return {tenant_name, job}
`

const (
	legacyQueueKey        = "nebi:jobs"
	pendingReconcileAge   = 30 * time.Second
	pendingReconcileBatch = 500
	tenantQueueKeyGlob    = "nebi:{jobs}:tenant:*"
)

// ValkeyQueue implements a distributed job queue using Valkey
// Valkey is used for job transport (job IDs only), DB is source of truth
type ValkeyQueue struct {
	client     valkey.Client
	db         *gorm.DB
	key        string // Queue key prefix: "nebi:jobs"
	tenantsKey string
	seqKey     string
	queuedKey  string
}

// NewValkeyQueue creates a new Valkey-backed queue
func NewValkeyQueue(addr string, db *gorm.DB) (*ValkeyQueue, error) {
	if db == nil {
		return nil, fmt.Errorf("database instance is required for Valkey queue")
	}

	// Create Valkey client with connection pool
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr},
		// Avoid client-side caching for queue keys; tests use miniredis and
		// queue state is mutated by Lua scripts rather than cacheable reads.
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
		queuedKey:  "nebi:{jobs}:queued",
	}

	if err := q.seedQueuedSet(ctx); err != nil {
		client.Close()
		return nil, err
	}
	if err := q.reconcilePendingJobs(ctx, pendingReconcileAge); err != nil {
		client.Close()
		return nil, err
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

	if err := q.enqueueTransport(ctx, job); err != nil {
		return err
	}

	slog.Debug("Job enqueued",
		"job_id", job.ID,
		"type", job.Type,
		"tenant", tenantKeyForJob(job),
		"queue_key", q.tenantQueueKey(tenantKeyForJob(job)))
	return nil
}

func (q *ValkeyQueue) enqueueTransport(ctx context.Context, job *models.Job) error {
	tenant := tenantKeyForJob(job)
	jobData, err := json.Marshal(map[string]string{
		"id": job.ID.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal job data: %w", err)
	}

	cmd := q.client.B().Eval().
		Script(enqueueTenantJobScript).
		Numkeys(4).
		Key(q.tenantQueueKey(tenant), q.tenantsKey, q.seqKey, q.queuedKey).
		Arg(string(jobData), tenant).
		Build()
	if err := q.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("failed to push job to Valkey: %w", err)
	}
	return nil
}

// Dequeue retrieves the next job from the queue. The fair-queue pop is a Lua
// script, so it cannot use server-side blocking primitives like BLPOP; poll
// briefly until either a job appears or the dequeue deadline expires.
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
		if job.Status != models.JobStatusPending {
			slog.Warn("Skipping non-pending job from Valkey transport", "job_id", job.ID, "status", job.Status)
			continue
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
		Numkeys(3).
		Key(q.tenantsKey, q.seqKey, q.queuedKey).
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
	jobID, err := decodeJobID(encoded)
	if err != nil {
		return nil, err
	}

	var job models.Job
	if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch job from database: %w", err)
	}
	return &job, nil
}

func decodeJobID(encoded string) (uuid.UUID, error) {
	var jobData map[string]string
	if err := json.Unmarshal([]byte(encoded), &jobData); err == nil {
		jobID, err := uuid.Parse(jobData["id"])
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to parse job ID: %w", err)
		}
		return jobID, nil
	}

	var rawID string
	if err := json.Unmarshal([]byte(encoded), &rawID); err == nil {
		encoded = rawID
	}
	jobID, err := uuid.Parse(strings.TrimSpace(encoded))
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to parse job ID: %w", err)
	}
	return jobID, nil
}

func (q *ValkeyQueue) seedQueuedSet(ctx context.Context) error {
	if err := q.client.Do(ctx, q.client.B().Del().Key(q.queuedKey).Build()).Error(); err != nil {
		return fmt.Errorf("failed to reset queued set: %w", err)
	}
	keys, err := q.client.Do(ctx, q.client.B().Keys().Pattern(tenantQueueKeyGlob).Build()).AsStrSlice()
	if err != nil {
		return fmt.Errorf("failed to list tenant queues: %w", err)
	}
	for _, key := range keys {
		values, err := q.client.Do(ctx, q.client.B().Lrange().Key(key).Start(0).Stop(-1).Build()).AsStrSlice()
		if err != nil {
			return fmt.Errorf("failed to scan tenant queue %s: %w", key, err)
		}
		if len(values) == 0 {
			continue
		}
		if err := q.client.Do(ctx, q.client.B().Sadd().Key(q.queuedKey).Member(values...).Build()).Error(); err != nil {
			return fmt.Errorf("failed to seed queued set: %w", err)
		}
	}
	return nil
}

func (q *ValkeyQueue) reconcilePendingJobs(ctx context.Context, minAge time.Duration) error {
	drained, err := q.drainLegacyQueue(ctx)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-minAge)
	requeued := 0
	for offset := 0; ; offset += pendingReconcileBatch {
		var jobs []models.Job
		if err := q.db.WithContext(ctx).
			Where("status = ? AND created_at <= ?", models.JobStatusPending, cutoff).
			Order("created_at ASC, id ASC").
			Limit(pendingReconcileBatch).
			Offset(offset).
			Find(&jobs).Error; err != nil {
			return fmt.Errorf("failed to load stale pending jobs: %w", err)
		}
		for i := range jobs {
			if err := q.enqueueTransport(ctx, &jobs[i]); err != nil {
				return fmt.Errorf("failed to re-enqueue pending job %s: %w", jobs[i].ID, err)
			}
		}
		requeued += len(jobs)
		if len(jobs) < pendingReconcileBatch {
			break
		}
	}
	if drained > 0 || requeued > 0 {
		slog.Info("Reconciled Valkey job transport", "legacy_jobs", drained, "pending_jobs", requeued)
	}
	return nil
}

func (q *ValkeyQueue) drainLegacyQueue(ctx context.Context) (int, error) {
	drained := 0
	for {
		encoded, err := q.client.Do(ctx, q.client.B().Lpop().Key(legacyQueueKey).Build()).ToString()
		if valkey.IsValkeyNil(err) {
			return drained, nil
		}
		if err != nil {
			return drained, fmt.Errorf("failed to pop legacy queue: %w", err)
		}
		jobID, err := decodeJobID(encoded)
		if err != nil {
			slog.Warn("Skipping invalid legacy Valkey job entry", "entry", encoded, "error", err)
			continue
		}
		var job models.Job
		if err := q.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				slog.Warn("Skipping legacy Valkey job entry with no DB row", "job_id", jobID)
				continue
			}
			return drained, fmt.Errorf("failed to fetch legacy job %s: %w", jobID, err)
		}
		if job.Status != models.JobStatusPending {
			continue
		}
		if err := q.enqueueTransport(ctx, &job); err != nil {
			return drained, fmt.Errorf("failed to migrate legacy job %s: %w", job.ID, err)
		}
		drained++
	}
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
