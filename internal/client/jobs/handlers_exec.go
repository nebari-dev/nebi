package jobs

// Owner: U2 (detail).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/models"
)

func init() {
	register(models.JobTypeInstallVersion, func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Exec.RunInstallVersion(ctx, rc.Job, rc.Log)
	})
	register(models.JobTypeUninstall, func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Exec.RunUninstall(ctx, rc.Job, rc.Log)
	})
}
