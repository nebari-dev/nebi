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
	_ "github.com/nebari-dev/nebi/internal/pkgmgr/uv" // Register uv
)

// LocalExecutor runs operations on the local machine
type LocalExecutor struct {
	baseDir string // Base directory for environments (e.g., /var/lib/nebi/environments)
	config  *config.Config
}

// NewLocalExecutor creates a new local executor
func NewLocalExecutor(cfg *config.Config) (*LocalExecutor, error) {
	baseDir := cfg.Storage.EnvironmentsDir

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

// GetEnvironmentPath returns the filesystem path for an environment
// Format: {baseDir}/{normalized-name}-{uuid}
func (e *LocalExecutor) GetEnvironmentPath(env *models.Environment) string {
	normalizedName := normalizeEnvName(env.Name)
	dirName := fmt.Sprintf("%s-%s", normalizedName, env.ID.String())
	return filepath.Join(e.baseDir, dirName)
}

// CreateEnvironment creates a new environment on the local filesystem
func (e *LocalExecutor) CreateEnvironment(ctx context.Context, env *models.Environment, logWriter io.Writer, pixiToml ...string) error {
	envPath := e.GetEnvironmentPath(env)

	fmt.Fprintf(logWriter, "Creating environment at: %s\n", envPath)

	// Create package manager instance
	pmType := env.PackageManager
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
			Name:      env.Name,
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

// InstallPackages installs packages in an environment
func (e *LocalExecutor) InstallPackages(ctx context.Context, env *models.Environment, packages []string, logWriter io.Writer) error {
	envPath := e.GetEnvironmentPath(env)

	fmt.Fprintf(logWriter, "Installing packages: %v\n", packages)

	// Get package manager
	pmType := env.PackageManager
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

// RemovePackages removes packages from an environment
func (e *LocalExecutor) RemovePackages(ctx context.Context, env *models.Environment, packages []string, logWriter io.Writer) error {
	envPath := e.GetEnvironmentPath(env)

	fmt.Fprintf(logWriter, "Removing packages: %v\n", packages)

	// Get package manager
	pmType := env.PackageManager
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

// DeleteEnvironment removes an environment from the filesystem
func (e *LocalExecutor) DeleteEnvironment(ctx context.Context, env *models.Environment, logWriter io.Writer) error {
	envPath := e.GetEnvironmentPath(env)

	fmt.Fprintf(logWriter, "Deleting environment at: %s\n", envPath)

	if err := os.RemoveAll(envPath); err != nil {
		return fmt.Errorf("failed to delete environment: %w", err)
	}

	fmt.Fprintf(logWriter, "Environment deleted successfully\n")
	return nil
}
