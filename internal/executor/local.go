package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"github.com/nebari-dev/nebi/internal/pkgmgr/pixi"
	"github.com/nebari-dev/nebi/internal/sandbox"
)

// LocalExecutor runs operations on the local machine
type LocalExecutor struct {
	baseDir string // Base directory for workspaces (e.g., /var/lib/nebi/environments)
	config  *config.Config
	sandbox *sandbox.Runner
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

	// Builds run untrusted, user-supplied manifests. Every pixi subprocess
	// this executor spawns goes through the sandbox runner, which scrubs the
	// environment down to an allowlist and (unless mode is "off") confines
	// the process to the workspace directory.
	sandboxMode := cfg.Sandbox.Mode
	if sandboxMode == "" {
		sandboxMode = "off"
	}
	sb, err := sandbox.NewRunner(sandbox.Config{
		Mode:         sandbox.Mode(sandboxMode),
		AllowedPorts: cfg.Sandbox.AllowedPorts,
	}, "")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize build sandbox: %w", err)
	}

	return &LocalExecutor{
		baseDir: baseDir,
		config:  cfg,
		sandbox: sb,
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

// StagingRoot returns {baseDir}/.import-staging, creating it if missing.
func (e *LocalExecutor) StagingRoot() string {
	root := filepath.Join(e.baseDir, ".import-staging")
	_ = os.MkdirAll(root, 0o755)
	return root
}

// GetWorkspacePath returns the filesystem path for a workspace.
// If ws.Path is set to an absolute path, prefer it regardless of source so
// reads/writes remain stable across process restarts or base-dir changes.
// Otherwise: {baseDir}/{normalized-name}-{uuid}
func (e *LocalExecutor) GetWorkspacePath(ws *models.Workspace) string {
	if ws.Path != "" && filepath.IsAbs(ws.Path) {
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

	pm, err := e.packageManagerFor(ws)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}
	fmt.Fprintf(logWriter, "Using package manager: %s\n", pm.Name())

	switch {
	case opts.SeedDir != "":
		// Always clean up the staging dir, even on partial failure (e.g. a
		// mid-walk error in seedWorkspaceFromDir leaves files behind).
		// Owned by this branch end-to-end.
		defer func() {
			if rmErr := os.RemoveAll(opts.SeedDir); rmErr != nil {
				fmt.Fprintf(logWriter, "Warning: staging cleanup failed: %v\n", rmErr)
			}
		}()
		if err := os.MkdirAll(envPath, 0o755); err != nil {
			return fmt.Errorf("create env dir: %w", err)
		}
		fmt.Fprintf(logWriter, "Seeding workspace from %s\n", opts.SeedDir)
		if err := seedWorkspaceFromDir(opts.SeedDir, envPath); err != nil {
			return fmt.Errorf("seed workspace: %w", err)
		}
		if err := runPixiLock(ctx, e.sandbox, pm, envPath, logWriter); err != nil {
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
		if err := runPixiLock(ctx, e.sandbox, pm, envPath, logWriter); err != nil {
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

// packageManagerFor resolves the package manager for a workspace, honoring
// the configured default type and explicit binary paths.
//
// Server-side builds evaluate user-supplied manifests, so the resulting
// manager is wired to the build sandbox: every pixi subprocess it spawns
// runs with a scrubbed environment and, unless the sandbox is off, confined
// to the workspace directory.
func (e *LocalExecutor) packageManagerFor(ws *models.Workspace) (pkgmgr.PackageManager, error) {
	pmType := ws.PackageManager
	if pmType == "" {
		pmType = e.config.PackageManager.DefaultType
	}

	var (
		pm  pkgmgr.PackageManager
		err error
	)
	switch {
	case pmType == "pixi" && e.config.PackageManager.PixiPath != "":
		pm, err = pkgmgr.NewWithPath(pmType, e.config.PackageManager.PixiPath)
	case pmType == "uv" && e.config.PackageManager.UvPath != "":
		pm, err = pkgmgr.NewWithPath(pmType, e.config.PackageManager.UvPath)
	default:
		pm, err = pkgmgr.New(pmType)
	}
	if err != nil {
		return nil, err
	}

	if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
		pixiMgr.SetSandbox(e.sandbox)
	}
	return pm, nil
}

// runPixiLock runs `pixi lock` in envPath. It resolves the dependency
// graph and writes pixi.lock without downloading or extracting packages;
// installing is a separate, explicit step (see InstallEnvironment).
func runPixiLock(ctx context.Context, sb *sandbox.Runner, pm pkgmgr.PackageManager, envPath string, logWriter io.Writer) error {
	pixiBinary := "pixi"
	if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
		pixiBinary = pixiMgr.BinaryPath()
	}
	lockCmd, err := sb.Command(ctx, sandbox.Spec{
		WorkspaceDir: envPath,
		Argv:         []string{pixiBinary, "lock"},
	})
	if err != nil {
		return fmt.Errorf("failed to prepare sandboxed pixi lock: %w", err)
	}
	lockCmd.Stdout = logWriter
	lockCmd.Stderr = logWriter
	fmt.Fprintf(logWriter, "Running: %s lock\n", pixiBinary)
	if err := lockCmd.Run(); err != nil {
		if sandbox.IsSetupFailure(err) {
			return fmt.Errorf("build sandbox setup failed: %w", err)
		}
		return fmt.Errorf("failed to lock pixi environment: %w", err)
	}
	fmt.Fprintf(logWriter, "Lockfile resolved successfully\n")
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

	pm, err := e.packageManagerFor(ws)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	opts := pkgmgr.InstallOptions{
		EnvPath:   envPath,
		Packages:  packages,
		LogWriter: logWriter,
		NoInstall: true,
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

	pm, err := e.packageManagerFor(ws)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	opts := pkgmgr.RemoveOptions{
		EnvPath:   envPath,
		Packages:  packages,
		LogWriter: logWriter,
		NoInstall: true,
	}

	if err := pm.Remove(ctx, opts); err != nil {
		return fmt.Errorf("failed to remove packages: %w", err)
	}

	fmt.Fprintf(logWriter, "Packages removed successfully\n")
	return nil
}

// SolveEnvironment runs pixi lock to resolve the current pixi.toml into
// pixi.lock. It never installs packages.
func (e *LocalExecutor) SolveEnvironment(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error {
	envPath := e.GetWorkspacePath(ws)

	fmt.Fprintf(logWriter, "Running pixi lock to solve environment...\n")

	pm, err := e.packageManagerFor(ws)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	if err := runPixiLock(ctx, e.sandbox, pm, envPath, logWriter); err != nil {
		return err
	}

	fmt.Fprintf(logWriter, "Environment solved successfully\n")
	return nil
}

// InstallEnvironment runs `pixi install -v` in the workspace directory,
// materializing .pixi/envs/ from the already-resolved pixi.lock.
func (e *LocalExecutor) InstallEnvironment(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error {
	envPath := e.GetWorkspacePath(ws)

	pm, err := e.packageManagerFor(ws)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	pixiBinary := "pixi"
	if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
		pixiBinary = pixiMgr.BinaryPath()
	}
	cmd, err := e.sandbox.Command(ctx, sandbox.Spec{
		WorkspaceDir: envPath,
		Argv:         []string{pixiBinary, "install", "-v"},
	})
	if err != nil {
		return fmt.Errorf("failed to prepare sandboxed pixi install: %w", err)
	}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	fmt.Fprintf(logWriter, "Running: %s install -v\n", pixiBinary)
	if err := cmd.Run(); err != nil {
		if sandbox.IsSetupFailure(err) {
			return fmt.Errorf("build sandbox setup failed: %w", err)
		}
		return fmt.Errorf("pixi install failed: %w", err)
	}
	fmt.Fprintf(logWriter, "Environment installed successfully\n")
	return nil
}

// ListPackages returns the packages installed in the workspace. It goes
// through packageManagerFor so the listing honors the configured package
// manager binary and runs inside the build sandbox: `pixi list` parses a
// user-supplied manifest and lockfile, so it is as untrusted as a build.
func (e *LocalExecutor) ListPackages(ctx context.Context, ws *models.Workspace) ([]pkgmgr.Package, error) {
	pm, err := e.packageManagerFor(ws)
	if err != nil {
		return nil, fmt.Errorf("failed to create package manager: %w", err)
	}
	return pm.List(ctx, pkgmgr.ListOptions{EnvPath: e.GetWorkspacePath(ws)})
}

// UninstallEnvironment removes the installed environment (.pixi/envs)
// from the workspace directory. Manifest and lockfile are untouched.
func (e *LocalExecutor) UninstallEnvironment(ctx context.Context, ws *models.Workspace, logWriter io.Writer) error {
	envsDir := filepath.Join(e.GetWorkspacePath(ws), ".pixi", "envs")
	fmt.Fprintf(logWriter, "Removing installed environment at: %s\n", envsDir)
	if err := os.RemoveAll(envsDir); err != nil {
		return fmt.Errorf("failed to remove installed environment: %w", err)
	}
	fmt.Fprintf(logWriter, "Environment uninstalled successfully\n")
	return nil
}

// IsEnvInstalled reports whether the workspace has an installed
// environment on disk (.pixi/envs exists).
func (e *LocalExecutor) IsEnvInstalled(ws *models.Workspace) bool {
	info, err := os.Stat(filepath.Join(e.GetWorkspacePath(ws), ".pixi", "envs"))
	return err == nil && info.IsDir()
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
