package jobs

// Owner: U7 (publish).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/models"
)

func init() {
	register(models.JobTypePullRegistry, func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Registry.RunPull(ctx, rc.Job, rc.Log)
	})
}
