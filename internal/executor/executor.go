package executor

import (
	"context"
	"io"

	"github.com/nebari-dev/nebi/internal/models"
)

// CreateProjectOptions tunes CreateProject. PixiToml seeds a newly
// created project with a pinned manifest. SeedDir seeds a newly
// created project from a pre-populated directory (used for bundle
// imports); when non-empty, PixiToml is ignored because the seed's
// pixi.toml is authoritative. SeedDir is removed after a successful
// create to keep staging from leaking.
type CreateProjectOptions struct {
	PixiToml string
	SeedDir  string
}

// Executor interface for running project operations
type Executor interface {
	CreateProject(ctx context.Context, project *models.Project, logWriter io.Writer, opts CreateProjectOptions) error
	InstallPackages(ctx context.Context, project *models.Project, packages []string, logWriter io.Writer) error
	RemovePackages(ctx context.Context, project *models.Project, packages []string, logWriter io.Writer) error
	DeleteProject(ctx context.Context, project *models.Project, logWriter io.Writer) error
	SolveEnvironment(ctx context.Context, project *models.Project, logWriter io.Writer) error
	CleanupJobArtifacts(ctx context.Context, project *models.Project, jobType models.JobType, logWriter io.Writer) error
	// InstallEnvironment materializes .pixi/envs from the resolved lockfile
	// (pixi install). UninstallEnvironment removes .pixi/envs, leaving
	// manifest and lockfile intact. IsEnvInstalled reports whether
	// .pixi/envs exists on disk.
	InstallEnvironment(ctx context.Context, project *models.Project, logWriter io.Writer) error
	UninstallEnvironment(ctx context.Context, project *models.Project, logWriter io.Writer) error
	IsEnvInstalled(project *models.Project) bool
	GetProjectPath(project *models.Project) string
	// StagingRoot returns a directory under the executor's storage root
	// suitable for one-off staging (e.g. bundle import pre-extraction).
	// The directory is ensured to exist.
	StagingRoot() string
}
