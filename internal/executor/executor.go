package executor

import (
	"context"
	"io"

	"github.com/nebari-dev/nebi/internal/models"
)

// Executor interface for running workspace operations
type Executor interface {
	// CreateWorkspace creates a new workspace with optional pixi.toml content
	CreateWorkspace(ctx context.Context, ws *models.Workspace, logWriter io.Writer, pixiToml ...string) error

	// InstallPackages installs packages in a workspace
	InstallPackages(ctx context.Context, ws *models.Workspace, packages []string, logWriter io.Writer) error

	// RemovePackages removes packages from a workspace
	RemovePackages(ctx context.Context, ws *models.Workspace, packages []string, logWriter io.Writer) error

	// DeleteWorkspace removes a workspace
	DeleteWorkspace(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error

	// GetWorkspacePath returns the filesystem path for a workspace
	GetWorkspacePath(ws *models.Workspace) string
}
