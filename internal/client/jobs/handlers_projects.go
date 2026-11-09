package jobs

// Owner: U1 (projects).

import (
	"context"
	"fmt"

	"github.com/nebari-dev/nebi/internal/models"
)

func init() {
	register(models.JobTypeCreate, runCreateProject)
	register(models.JobTypeDelete, func(ctx context.Context, rc RunContext) error {
		return rc.SDK.Local.RunDeleteProject(ctx, rc.Job, rc.Log)
	})
}

// runCreateProject creates the project and its version 1, then, for
// "Create and install" (metadata install=true), installs version 1 through
// the exec SDK's install_version job body. The chaining lives here because
// sdk/local must not import sdk/exec.
func runCreateProject(ctx context.Context, rc RunContext) error {
	if err := rc.SDK.Local.RunCreateProject(ctx, rc.Job, rc.Log); err != nil {
		return err
	}
	if !createWantsInstall(rc.Job) {
		return nil
	}
	if rc.Job.VersionNumber == nil {
		one := 1
		rc.Job.VersionNumber = &one
	}
	fmt.Fprintf(rc.Log, "Installing version %d\n", *rc.Job.VersionNumber)
	if err := rc.SDK.Exec.RunInstallVersion(ctx, rc.Job, rc.Log); err != nil {
		return fmt.Errorf("version 1 was created but couldn’t be installed: %w", err)
	}
	return nil
}

func createWantsInstall(job *models.Job) bool {
	switch v := job.Metadata["install"].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}
