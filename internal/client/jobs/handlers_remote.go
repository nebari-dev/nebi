package jobs

// Owner: U5 (remote).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/models"
)

func init() {
	run := func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Remote.RunPull(ctx, rc.Job, rc.Log)
	}
	register(models.JobTypePull, run)
	register(models.JobTypePullInstall, run)
}
