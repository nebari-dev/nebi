package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedWorkspaceEnvScopesWritableProcessDirs(t *testing.T) {
	envPath := t.TempDir()

	env, err := PreparedWorkspaceEnv(envPath)
	if err != nil {
		t.Fatalf("PreparedWorkspaceEnv: %v", err)
	}
	values := envMap(env)

	want := map[string]string{
		"TMPDIR":            filepath.Join(envPath, ".nebi", "tmp"),
		"TMP":               filepath.Join(envPath, ".nebi", "tmp"),
		"TEMP":              filepath.Join(envPath, ".nebi", "tmp"),
		"XDG_CACHE_HOME":    filepath.Join(envPath, ".nebi", "cache"),
		"HOME":              filepath.Join(envPath, ".nebi", "home"),
		"PIXI_CACHE_DIR":    filepath.Join(envPath, ".nebi", "pixi-cache"),
		"RATTLER_CACHE_DIR": filepath.Join(envPath, ".nebi", "pixi-cache"),
		"PIP_CACHE_DIR":     filepath.Join(envPath, ".nebi", "pixi-cache", "pip"),
		"UV_CACHE_DIR":      filepath.Join(envPath, ".nebi", "pixi-cache", "uv"),
	}
	for key, dir := range want {
		if values[key] != dir {
			t.Fatalf("%s = %q, want %q", key, values[key], dir)
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("%s dir was not prepared: info=%v err=%v", key, info, err)
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
