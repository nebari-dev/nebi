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
// package-manager processes.
func WorkspaceEnv(envPath string) []string {
	if envPath == "" {
		return nil
	}

	dirs := workspaceEnvDirs(envPath)
	return append(os.Environ(),
		"TMPDIR="+dirs.tmp,
		"TMP="+dirs.tmp,
		"TEMP="+dirs.tmp,
		"PIP_CACHE_DIR="+dirs.pipCache,
		"UV_CACHE_DIR="+dirs.uvCache,
	)
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
