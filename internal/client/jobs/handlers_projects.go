package jobs

// Owner: U1 (projects).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/models"
)

func init() {
	register(models.JobTypeCreate, func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Local.RunCreateProject(ctx, rc.Job, rc.Log)
	})
	register(models.JobTypeDelete, func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Local.RunDeleteProject(ctx, rc.Job, rc.Log)
	})
}
