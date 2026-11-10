package shared

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

// JobRequest asks the job system to run an operation on a project.
type JobRequest struct {
	ProjectID     uuid.UUID
	Type          models.JobType
	VersionNumber *int
	// Metadata is persisted on the job row and handed to the handler.
	// Keys are documented per job type in CONTRACTS.md.
	Metadata map[string]any
}

// JobSubmitter is how SDK code starts long-running work without importing
// internal/client/jobs (which imports the SDK). internal/client/jobs
// implements it; the stack installs it with Runtime.SetJobs.
//
// In nebi-web and the desktop app Submit enqueues the job on the worker
// queue and returns at once (status "pending"). In the CLI it runs the job
// inline, streaming the log to the terminal, and returns the finished job.
type JobSubmitter interface {
	Submit(ctx context.Context, req JobRequest) (*models.Job, error)
}
