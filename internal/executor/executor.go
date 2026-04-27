package executor

import (
	"context"
	"io"

	"github.com/nebari-dev/nebi/internal/models"
)

// CreateWorkspaceOptions tunes CreateWorkspace. PixiToml seeds a newly
// created workspace with a pinned manifest. SeedDir seeds a newly
// created workspace from a pre-populated directory (used for bundle
// imports); when non-empty, PixiToml is ignored because the seed's
// pixi.toml is authoritative. SeedDir is removed after a successful
// create to keep staging from leaking.
type CreateWorkspaceOptions struct {
	PixiToml string
	SeedDir  string
}

// Executor interface for running workspace operations
type Executor interface {
	CreateWorkspace(ctx context.Context, ws *models.Workspace, logWriter io.Writer, opts CreateWorkspaceOptions) error
	InstallPackages(ctx context.Context, ws *models.Workspace, packages []string, logWriter io.Writer) error
	RemovePackages(ctx context.Context, ws *models.Workspace, packages []string, logWriter io.Writer) error
	DeleteWorkspace(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error
	SolveEnvironment(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error
	GetWorkspacePath(ws *models.Workspace) string
	// StagingRoot returns a directory under the executor's storage root
	// suitable for one-off staging (e.g. bundle import pre-extraction).
	// The directory is ensured to exist.
	StagingRoot() string
}
