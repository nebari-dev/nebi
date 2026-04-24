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
func (e *LocalExecutor) CreateWorkspace(ctx context.Context, ws *models.Workspace, logWriter io.Writer, opts CreateWorkspaceOptions) error {
	envPath := e.GetWorkspacePath(ws)
	fmt.Fprintf(logWriter, "Creating environment at: %s\n", envPath)

	pmType := ws.PackageManager
	if pmType == "" {
		pmType = e.config.PackageManager.DefaultType
	}

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

	switch {
	case opts.SeedDir != "":
		if err := os.MkdirAll(envPath, 0o755); err != nil {
			return fmt.Errorf("create env dir: %w", err)
		}
		fmt.Fprintf(logWriter, "Seeding workspace from %s\n", opts.SeedDir)
		if err := seedWorkspaceFromDir(opts.SeedDir, envPath); err != nil {
			return fmt.Errorf("seed workspace: %w", err)
		}
		if err := os.RemoveAll(opts.SeedDir); err != nil {
			fmt.Fprintf(logWriter, "Warning: staging cleanup failed: %v\n", err)
		}
		if err := runPixiInstall(ctx, pm, envPath, logWriter); err != nil {
			return err
		}

	case opts.PixiToml != "":
		if err := os.MkdirAll(envPath, 0o755); err != nil {
			return fmt.Errorf("create env dir: %w", err)
		}
		pixiTomlPath := filepath.Join(envPath, "pixi.toml")
		fmt.Fprintf(logWriter, "Writing custom pixi.toml content\n")
		if err := os.WriteFile(pixiTomlPath, []byte(opts.PixiToml), 0o644); err != nil {
			return fmt.Errorf("failed to write pixi.toml: %w", err)
		}
		if err := runPixiInstall(ctx, pm, envPath, logWriter); err != nil {
			return err
		}

	default:
		initOpts := pkgmgr.InitOptions{
			EnvPath:   envPath,
			Name:      ws.Name,
			Channels:  []string{"conda-forge"},
			LogWriter: logWriter,
		}
		if err := pm.Init(ctx, initOpts); err != nil {
			return fmt.Errorf("failed to initialize environment: %w", err)
		}
	}

	fmt.Fprintf(logWriter, "Environment created successfully\n")
	return nil
}

// runPixiInstall runs `pixi install -v` in envPath. Extracted so the
// pixi.toml-only and seed-dir branches share the install step.
func runPixiInstall(ctx context.Context, pm pkgmgr.PackageManager, envPath string, logWriter io.Writer) error {
	pixiBinary := "pixi"
	if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
		pixiBinary = pixiMgr.BinaryPath()
	}
	installCmd := exec.CommandContext(ctx, pixiBinary, "install", "-v")
	installCmd.Dir = envPath
	installCmd.Stdout = logWriter
	installCmd.Stderr = logWriter
	fmt.Fprintf(logWriter, "Running: %s install -v\n", pixiBinary)
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("failed to install pixi environment: %w", err)
	}
	fmt.Fprintf(logWriter, "Pixi environment installed successfully\n")
	return nil
}

// seedWorkspaceFromDir recursively moves every entry under srcDir into
// dstDir, preserving relative paths. Rejects any cleaned relative path
// that escapes dstDir as defense-in-depth. Uses os.Rename when possible;
// falls back to copy when the rename fails (cross-filesystem, etc.).
func seedWorkspaceFromDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		cleaned := filepath.Clean(rel)
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || filepath.IsAbs(cleaned) {
			return fmt.Errorf("unsafe seed path %q", rel)
		}
		dst := filepath.Join(dstDir, cleaned)

		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(path, dst); err == nil {
			return nil
		}
		// Fallback: copy bytes.
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
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

// SolveEnvironment runs pixi install to solve and install the current pixi.toml
func (e *LocalExecutor) SolveEnvironment(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error {
	envPath := e.GetWorkspacePath(ws)

	fmt.Fprintf(logWriter, "Running pixi install to solve environment...\n")

	// Get pixi binary path
	pmType := ws.PackageManager
	if pmType == "" {
		pmType = e.config.PackageManager.DefaultType
	}

	pixiBinary := "pixi"
	if pmType == "pixi" && e.config.PackageManager.PixiPath != "" {
		pm, err := pkgmgr.NewWithPath(pmType, e.config.PackageManager.PixiPath)
		if err == nil {
			if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
				pixiBinary = pixiMgr.BinaryPath()
			}
		}
	}

	cmd := exec.CommandContext(ctx, pixiBinary, "install", "-v")
	cmd.Dir = envPath
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	fmt.Fprintf(logWriter, "Running: pixi install -v\n")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pixi install failed: %w", err)
	}

	fmt.Fprintf(logWriter, "Environment solved and installed successfully\n")
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
