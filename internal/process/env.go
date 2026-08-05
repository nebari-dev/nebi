package process

import (
	"fmt"
	"os"
	"path/filepath"
)

// PreparedWorkspaceEnv keeps package-manager temp/cache/home writes under the
// workspace so storage accounting observes them.
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
		"XDG_CACHE_HOME="+dirs.xdgCache,
		"HOME="+dirs.home,
		"PIXI_CACHE_DIR="+dirs.pixiCache,
		"RATTLER_CACHE_DIR="+dirs.pixiCache,
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

func prepareWorkspaceEnvDirs(envPath string) error {
	if envPath == "" {
		return nil
	}
	dirs := workspaceEnvDirs(envPath)
	for _, dir := range []string{dirs.tmp, dirs.xdgCache, dirs.home, dirs.pixiCache, dirs.pipCache, dirs.uvCache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("prepare workspace process dir %s: %w", dir, err)
		}
	}
	return nil
}
