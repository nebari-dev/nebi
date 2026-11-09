package jobs

import (
	"context"
	"fmt"
	"io"

	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/models"
)

// Dispatcher routes client-submitted jobs to their registered handler. It
// implements worker.Dispatcher and localext.Dispatcher.
type Dispatcher struct {
	sdk *sdk.Client
}

// NewDispatcher builds a dispatcher over the SDK.
func NewDispatcher(c *sdk.Client) *Dispatcher {
	return &Dispatcher{sdk: c}
}

// IsClientJob reports whether the job was submitted by the client stack.
func IsClientJob(job *models.Job) bool {
	if job == nil || job.Metadata == nil {
		return false
	}
	origin, _ := job.Metadata[MetadataOrigin].(string)
	return origin == OriginClient
}

// Handles reports whether the job is a client job with a registered handler.
func (d *Dispatcher) Handles(job *models.Job) bool {
	if !IsClientJob(job) {
		return false
	}
	_, ok := handlerFor(job.Type)
	return ok
}

// Run executes the job's handler.
func (d *Dispatcher) Run(ctx context.Context, job *models.Job, logWriter io.Writer) error {
	h, ok := handlerFor(job.Type)
	if !ok {
		return fmt.Errorf("no client handler for job type %q", job.Type)
	}
	return h(ctx, RunContext{SDK: d.sdk, Job: job, Log: logWriter})
}
