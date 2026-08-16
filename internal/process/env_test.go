package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedWorkspaceEnvScopesWritableProcessDirs(t *testing.T) {
	envPath := t.TempDir()
	homeDir := t.TempDir()
	xdgCacheDir := t.TempDir()
	pixiCacheDir := t.TempDir()
	rattlerCacheDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CACHE_HOME", xdgCacheDir)
	t.Setenv("PIXI_CACHE_DIR", pixiCacheDir)
	t.Setenv("RATTLER_CACHE_DIR", rattlerCacheDir)

	env, err := PreparedWorkspaceEnv(envPath)
	if err != nil {
		t.Fatalf("PreparedWorkspaceEnv: %v", err)
	}
	values := envMap(env)

	want := map[string]string{
		"TMPDIR":        filepath.Join(envPath, ".nebi", "tmp"),
		"TMP":           filepath.Join(envPath, ".nebi", "tmp"),
		"TEMP":          filepath.Join(envPath, ".nebi", "tmp"),
		"PIP_CACHE_DIR": filepath.Join(envPath, ".nebi", "pixi-cache", "pip"),
		"UV_CACHE_DIR":  filepath.Join(envPath, ".nebi", "pixi-cache", "uv"),
	}
	for key, dir := range want {
		if values[key] != dir {
			t.Fatalf("%s = %q, want %q", key, values[key], dir)
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("%s dir was not prepared: info=%v err=%v", key, info, err)
		}
	}

	preserved := map[string]string{
		"HOME":              homeDir,
		"XDG_CACHE_HOME":    xdgCacheDir,
		"PIXI_CACHE_DIR":    pixiCacheDir,
		"RATTLER_CACHE_DIR": rattlerCacheDir,
	}
	for key, want := range preserved {
		if values[key] != want {
			t.Fatalf("%s = %q, want preserved value %q", key, values[key], want)
		}
	}
}

func envMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		for i := 0; i < len(entry); i++ {
			if entry[i] == '=' {
				out[entry[:i]] = entry[i+1:]
				break
			}
		}
	}
	return out
}
