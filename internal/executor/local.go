package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"github.com/nebari-dev/nebi/internal/pkgmgr/pixi"
)

// LocalExecutor runs operations on the local machine
type LocalExecutor struct {
	baseDir string // Base directory for workspaces (e.g., /var/lib/nebi/environments)
	config  *config.Config
}

// NewLocalExecutor creates a new local executor
func NewLocalExecutor(cfg *config.Config) (*LocalExecutor, error) {
	baseDir := cfg.Storage.WorkspacesDir

	// Resolve to absolute path so stored paths work from any working directory
	if !filepath.IsAbs(baseDir) {
		abs, err := filepath.Abs(baseDir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve base directory: %w", err)
		}
		baseDir = abs
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &LocalExecutor{
		baseDir: baseDir,
		config:  cfg,
	}, nil
}

// normalizeEnvName converts environment name to a filesystem-safe format
func normalizeEnvName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)
	// Replace spaces and special characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	name = reg.ReplaceAllString(name, "-")
	// Trim hyphens from start and end
	name = strings.Trim(name, "-")
	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// GetWorkspacePath returns the filesystem path for a workspace
// For source=="local" workspaces with a path set, returns that path directly.
// Otherwise: {baseDir}/{normalized-name}-{uuid}
func (e *LocalExecutor) GetWorkspacePath(ws *models.Workspace) string {
	if ws.Source == "local" && ws.Path != "" {
		return ws.Path
	}
	normalizedName := normalizeEnvName(ws.Name)
	dirName := fmt.Sprintf("%s-%s", normalizedName, ws.ID.String())
	return filepath.Join(e.baseDir, dirName)
}

// CreateWorkspace creates a new workspace on the local filesystem
func (e *LocalExecutor) CreateWorkspace(ctx context.Context, ws *models.Workspace, logWriter io.Writer, pixiToml ...string) error {
	envPath := e.GetWorkspacePath(ws)

	fmt.Fprintf(logWriter, "Creating environment at: %s\n", envPath)

	// Create package manager instance
	pmType := ws.PackageManager
	if pmType == "" {
		pmType = e.config.PackageManager.DefaultType
	}

	// Use custom path if configured
	var pm pkgmgr.PackageManager
	var err error
	if pmType == "pixi" && e.config.PackageManager.PixiPath != "" {
		pm, err = pkgmgr.NewWithPath(pmType, e.config.PackageManager.PixiPath)
	} else if pmType == "uv" && e.config.PackageManager.UvPath != "" {
		pm, err = pkgmgr.NewWithPath(pmType, e.config.PackageManager.UvPath)
	} else {
		pm, err = pkgmgr.New(pmType)
	}

	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	fmt.Fprintf(logWriter, "Using package manager: %s\n", pm.Name())

	// Check if custom pixi.toml content is provided
	if len(pixiToml) > 0 && pixiToml[0] != "" {
		// Create environment directory
		if err := os.MkdirAll(envPath, 0755); err != nil {
			return fmt.Errorf("failed to create environment directory: %w", err)
		}

		// Write custom pixi.toml content
		pixiTomlPath := filepath.Join(envPath, "pixi.toml")
		fmt.Fprintf(logWriter, "Writing custom pixi.toml content\n")
		if err := os.WriteFile(pixiTomlPath, []byte(pixiToml[0]), 0644); err != nil {
			return fmt.Errorf("failed to write pixi.toml: %w", err)
		}
		fmt.Fprintf(logWriter, "Custom pixi.toml written successfully\n")

		// Run pixi install to create the actual environment
		// This will read the pixi.toml and create the .pixi directory with the conda environment
		fmt.Fprintf(logWriter, "Installing environment from pixi.toml\n")

		// Get pixi binary path from package manager
		pixiBinary := "pixi" // Default fallback
		if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
			pixiBinary = pixiMgr.BinaryPath()
		}

		installCmd := exec.CommandContext(ctx, pixiBinary, "install", "-v")
		installCmd.Dir = envPath
		installCmd.Stdout = logWriter
		installCmd.Stderr = logWriter

		fmt.Fprintf(logWriter, "Running: pixi install -v\n")
		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("failed to install pixi environment: %w", err)
		}
		fmt.Fprintf(logWriter, "Pixi environment installed successfully\n")
	} else {
		// Initialize environment with default configuration
		// Note: For pixi, the Name parameter is used as the project name in pixi.toml
		// The environment is created in EnvPath directory itself
		opts := pkgmgr.InitOptions{
			EnvPath:   envPath,
			Name:      ws.Name,
			Channels:  []string{"conda-forge"}, // Default channel for pixi
			LogWriter: logWriter,
		}

		if err := pm.Init(ctx, opts); err != nil {
			return fmt.Errorf("failed to initialize environment: %w", err)
		}
	}

	fmt.Fprintf(logWriter, "Environment created successfully\n")
	return nil
}

// InstallPackages installs packages in a workspace
func (e *LocalExecutor) InstallPackages(ctx context.Context, ws *models.Workspace, packages []string, logWriter io.Writer) error {
	envPath := e.GetWorkspacePath(ws)

	fmt.Fprintf(logWriter, "Installing packages: %v\n", packages)

	// Get package manager
	pmType := ws.PackageManager
	if pmType == "" {
		pmType = e.config.PackageManager.DefaultType
	}

	pm, err := pkgmgr.New(pmType)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	opts := pkgmgr.InstallOptions{
		EnvPath:   envPath,
		Packages:  packages,
		LogWriter: logWriter,
	}

	if err := pm.Install(ctx, opts); err != nil {
		return fmt.Errorf("failed to install packages: %w", err)
	}

	fmt.Fprintf(logWriter, "Packages installed successfully\n")
	return nil
}

// RemovePackages removes packages from a workspace
func (e *LocalExecutor) RemovePackages(ctx context.Context, ws *models.Workspace, packages []string, logWriter io.Writer) error {
	envPath := e.GetWorkspacePath(ws)

	fmt.Fprintf(logWriter, "Removing packages: %v\n", packages)

	// Get package manager
	pmType := ws.PackageManager
	if pmType == "" {
		pmType = e.config.PackageManager.DefaultType
	}

	pm, err := pkgmgr.New(pmType)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	opts := pkgmgr.RemoveOptions{
		EnvPath:   envPath,
		Packages:  packages,
		LogWriter: logWriter,
	}

	if err := pm.Remove(ctx, opts); err != nil {
		return fmt.Errorf("failed to remove packages: %w", err)
	}

	fmt.Fprintf(logWriter, "Packages removed successfully\n")
	return nil
}

// DeleteWorkspace removes a workspace from the filesystem.
// For source=="local" workspaces the directory belongs to the user, so we
// only deregister (the caller handles DB cleanup) and never touch the filesystem.
func (e *LocalExecutor) DeleteWorkspace(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error {
	if ws.Source == "local" {
		fmt.Fprintf(logWriter, "Local workspace %q — skipping filesystem deletion\n", ws.Name)
		return nil
	}

	envPath := e.GetWorkspacePath(ws)

	fmt.Fprintf(logWriter, "Deleting workspace at: %s\n", envPath)

	if err := os.RemoveAll(envPath); err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	fmt.Fprintf(logWriter, "Workspace deleted successfully\n")
	return nil
}
