package executor

import (
	"context"
	"io"

	"github.com/openteams-ai/darb/internal/models"
)

// Executor interface for running environment operations
type Executor interface {
	// CreateEnvironment creates a new environment with optional pixi.toml content
	CreateEnvironment(ctx context.Context, env *models.Environment, logWriter io.Writer, pixiToml ...string) error

	// InstallPackages installs packages in an environment
	InstallPackages(ctx context.Context, env *models.Environment, packages []string, logWriter io.Writer) error

	// RemovePackages removes packages from an environment
	RemovePackages(ctx context.Context, env *models.Environment, packages []string, logWriter io.Writer) error

	// DeleteEnvironment removes an environment
	DeleteEnvironment(ctx context.Context, env *models.Environment, logWriter io.Writer) error

	// GetEnvironmentPath returns the filesystem path for an environment
	GetEnvironmentPath(env *models.Environment) string
}
