package datadir

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefault(t *testing.T) {
	override := t.TempDir()
	t.Setenv(EnvDataDir, override)
	if got, err := Default(); err != nil || got != override {
		t.Fatalf("Default() with %s = %q, %v; want %s", EnvDataDir, got, err, override)
	}

	home := t.TempDir()
	t.Setenv(EnvDataDir, "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	appData := filepath.Join(home, "roaming")
	t.Setenv("APPDATA", appData)
	want := map[string]string{
		"darwin":  filepath.Join(home, "Library", "Application Support", "nebi"),
		"windows": filepath.Join(appData, "nebi"),
	}[runtime.GOOS]
	if want == "" {
		want = filepath.Join(home, ".local", "share", "nebi")
	}
	if got, err := Default(); err != nil || got != want {
		t.Fatalf("Default() = %q, %v; want %s", got, err, want)
	}
}
