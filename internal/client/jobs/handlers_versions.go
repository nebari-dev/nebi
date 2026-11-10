package jobs

// Owner: U3 (newversion).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/models"
)

func init() {
	register(models.JobTypeCreateVersion, func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Local.RunCreateVersion(ctx, rc.Job, rc.Log)
	})
}
