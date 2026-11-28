package datadir

import (
	"errors"
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

// Every system's path, on whichever system runs the tests.
func TestDefaultFor(t *testing.T) {
	const home = "/home/me"
	homeDir := func() (string, error) { return home, nil }
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	for name, tc := range map[string]struct {
		goos string
		vars map[string]string
		want string
	}{
		"macOS":               {"darwin", nil, filepath.Join(home, "Library", "Application Support", "nebi")},
		"Windows":             {"windows", map[string]string{"APPDATA": `C:\Users\me\AppData\Roaming`}, filepath.Join(`C:\Users\me\AppData\Roaming`, "nebi")},
		"Windows, no APPDATA": {"windows", nil, filepath.Join(home, "AppData", "Roaming", "nebi")},
		"Linux":               {"linux", nil, filepath.Join(home, ".local", "share", "nebi")},
		"FreeBSD":             {"freebsd", nil, filepath.Join(home, ".local", "share", "nebi")},
		"override":            {"darwin", map[string]string{EnvDataDir: "/data/nebi"}, "/data/nebi"},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := defaultFor(tc.goos, homeDir, env(tc.vars)); err != nil || got != tc.want {
				t.Fatalf("defaultFor(%s) = %q, %v; want %s", tc.goos, got, err, tc.want)
			}
		})
	}

	noHome := errors.New("no home")
	if _, err := defaultFor("linux", func() (string, error) { return "", noHome }, env(nil)); !errors.Is(err, noHome) {
		t.Fatalf("defaultFor without a home directory = %v, want its error", err)
	}
}
