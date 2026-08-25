package process

import (
	"fmt"
	"os"
	"path/filepath"
)

// PreparedWorkspaceEnv keeps package-manager temp and language-cache writes
// under the workspace while preserving HOME and Pixi/Rattler cache variables
// for user config, credentials, and shared package caches.
func PreparedWorkspaceEnv(envPath string) ([]string, error) {
	if err := prepareWorkspaceEnvDirs(envPath); err != nil {
		return nil, err
	}
	return WorkspaceEnv(envPath), nil
}

// WorkspaceEnv returns workspace-scoped environment overrides for child
// package-manager processes, layered over the parent environment.
//
// Callers that must not leak the parent environment to untrusted build code
// (see internal/sandbox) use WorkspaceEnvOverrides instead and layer it over
// their own allowlist.
func WorkspaceEnv(envPath string) []string {
	if envPath == "" {
		return nil
	}
	return append(os.Environ(), WorkspaceEnvOverrides(envPath)...)
}

// WorkspaceEnvOverrides returns only the workspace-scoped variables, without
// inheriting anything from the parent environment.
func WorkspaceEnvOverrides(envPath string) []string {
	if envPath == "" {
		return nil
	}

	dirs := workspaceEnvDirs(envPath)
	return []string{
		"TMPDIR=" + dirs.tmp,
		"TMP=" + dirs.tmp,
		"TEMP=" + dirs.tmp,
		"PIP_CACHE_DIR=" + dirs.pipCache,
		"UV_CACHE_DIR=" + dirs.uvCache,
	}
}

// WorkspaceHomeDir returns the workspace-scoped HOME. It is only used when the
// build sandbox is active; see internal/sandbox for why HOME is left alone
// otherwise. Kept here so the sandbox reuses this package's directory layout,
// which WorkspaceTransientDirs already knows how to clean up.
func WorkspaceHomeDir(envPath string) string {
	if envPath == "" {
		return ""
	}
	return workspaceEnvDirs(envPath).home
}

// WorkspaceTmpDir returns the workspace-scoped TMPDIR.
func WorkspaceTmpDir(envPath string) string {
	if envPath == "" {
		return ""
	}
	return workspaceEnvDirs(envPath).tmp
}

// PrepareWorkspaceDirs creates the workspace-owned process directories.
// PreparedWorkspaceEnv does this as a side effect; callers that build their
// own environment need it on its own.
func PrepareWorkspaceDirs(envPath string) error {
	return prepareWorkspaceEnvDirs(envPath)
}

type workspaceDirs struct {
	tmp       string
	xdgCache  string
	home      string
	pixiCache string
	pipCache  string
	uvCache   string
}

func workspaceEnvDirs(envPath string) workspaceDirs {
	nebiDir := filepath.Join(envPath, ".nebi")
	pixiCache := filepath.Join(nebiDir, "pixi-cache")
	return workspaceDirs{
		tmp:       filepath.Join(nebiDir, "tmp"),
		xdgCache:  filepath.Join(nebiDir, "cache"),
		home:      filepath.Join(nebiDir, "home"),
		pixiCache: pixiCache,
		pipCache:  filepath.Join(pixiCache, "pip"),
		uvCache:   filepath.Join(pixiCache, "uv"),
	}
}

// WorkspaceTransientDirs returns workspace-owned process artifacts that can be
// removed after failed or interrupted jobs. It includes legacy dirs from older
// env preparation so cleanup handles upgrades safely.
func WorkspaceTransientDirs(envPath string) []string {
	if envPath == "" {
		return nil
	}
	dirs := workspaceEnvDirs(envPath)
	return []string{dirs.pixiCache, dirs.tmp, dirs.xdgCache, dirs.home}
}

func prepareWorkspaceEnvDirs(envPath string) error {
	if envPath == "" {
		return nil
	}
	dirs := workspaceEnvDirs(envPath)
	for _, dir := range []string{dirs.tmp, dirs.pixiCache, dirs.pipCache, dirs.uvCache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("prepare workspace process dir %s: %w", dir, err)
		}
	}
	return nil
}
