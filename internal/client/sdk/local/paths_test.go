package local

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultProjectPath(t *testing.T) {
	t.Run("option wins", func(t *testing.T) {
		h := newHarness(t)
		got, err := h.svc.DefaultProjectPath("demo")
		if err != nil {
			t.Fatal(err)
		}
		if want := h.resolved(filepath.Join(h.projectsDir, "demo")); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
	t.Run("~/nebi-projects by default", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		// The SDK reads no environment variable; entrypoints pass
		// NEBI_PROJECTS_DIR as Options.ProjectsDir.
		t.Setenv("NEBI_PROJECTS_DIR", filepath.Join(t.TempDir(), "ignored"))
		h := newHarness(t, func(o *Options) { o.ProjectsDir = "" })
		got, err := h.svc.DefaultProjectPath("demo")
		if err != nil {
			t.Fatal(err)
		}
		if want := h.resolved(filepath.Join(home, "nebi-projects", "demo")); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
	t.Run("invalid name", func(t *testing.T) {
		h := newHarness(t)
		for _, n := range []string{"../escape", "Demo", ""} {
			if _, err := h.svc.DefaultProjectPath(n); !errors.Is(err, ErrInvalidName) {
				t.Errorf("%q: want ErrInvalidName, got %v", n, err)
			}
		}
	})
}

func TestResolveSymlinksOfPathThatDoesNotExistYet(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := resolveSymlinks(filepath.Join(link, "new", "child"))
	if err != nil {
		t.Fatal(err)
	}
	realResolved, _ := filepath.EvalSymlinks(real)
	if want := filepath.Join(realResolved, "new", "child"); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestNormalizePathExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := normalizePath("~/work/demo")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := resolveSymlinks(filepath.Join(home, "work", "demo"))
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestIsWithin(t *testing.T) {
	dir := filepath.FromSlash("/data/projects")
	cases := map[string]bool{
		"/data/projects/demo":       true,
		"/data/projects/a/b":        true,
		"/data/projects":            false,
		"/data/projects-other/demo": false,
		"/data":                     false,
		"/elsewhere/demo":           false,
	}
	for p, want := range cases {
		if got := isWithin(dir, filepath.FromSlash(p)); got != want {
			t.Errorf("isWithin(%s, %s) = %v, want %v", dir, p, got, want)
		}
	}
}

// caseInsensitive reports whether the filesystem holding dir ignores case
// in names, as macOS and Windows do by default.
func caseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "case-probe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(probe)
	_, err := os.Stat(filepath.Join(dir, "CASE-PROBE"))
	return err == nil
}

func TestSameDirAndInside(t *testing.T) {
	base := mustResolve(t, t.TempDir())
	outer := filepath.Join(base, "outer")
	if err := os.MkdirAll(filepath.Join(outer, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !sameDir(outer, outer) || sameDir(outer, filepath.Join(outer, "inner")) {
		t.Fatal("sameDir by path")
	}
	if !inside(outer, filepath.Join(outer, "inner")) || inside(outer, outer) || inside(filepath.Join(outer, "inner"), outer) {
		t.Fatal("inside by path")
	}
	if !inside(outer, filepath.Join(outer, "missing", "deeper")) {
		t.Fatal("a path that does not exist yet is inside its existing ancestor")
	}

	link := filepath.Join(base, "link")
	if err := os.Symlink(outer, link); err == nil {
		if !sameDir(outer, link) || !inside(outer, filepath.Join(link, "inner")) {
			t.Fatal("a symlink to a directory is the same directory")
		}
	}

	if caseInsensitive(t, base) {
		upper := filepath.Join(base, "OUTER")
		if !sameDir(outer, upper) || !inside(outer, filepath.Join(upper, "inner")) || !inside(upper, filepath.Join(outer, "inner")) {
			t.Fatal("paths that differ only in case are the same directory here")
		}
	} else if sameDir(outer, filepath.Join(base, "OUTER")) {
		t.Fatal("paths that differ in case are different directories here")
	}
}

func mustResolve(t *testing.T, p string) string {
	t.Helper()
	r, err := resolveSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
