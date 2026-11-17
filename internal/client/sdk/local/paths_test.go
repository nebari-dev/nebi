package local

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultProjectPath(t *testing.T) {
	t.Run("option wins", func(t *testing.T) {
		t.Setenv(ProjectsDirEnv, filepath.Join(t.TempDir(), "from-env"))
		h := newHarness(t)
		got, err := h.svc.DefaultProjectPath("demo")
		if err != nil {
			t.Fatal(err)
		}
		if want := h.resolved(filepath.Join(h.projectsDir, "demo")); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
	t.Run("NEBI_PROJECTS_DIR when no option", func(t *testing.T) {
		envDir := filepath.Join(t.TempDir(), "from-env")
		t.Setenv(ProjectsDirEnv, envDir)
		h := newHarness(t, func(o *Options) { o.ProjectsDir = "" })
		got, err := h.svc.DefaultProjectPath("demo")
		if err != nil {
			t.Fatal(err)
		}
		if want := h.resolved(filepath.Join(envDir, "demo")); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
	t.Run("~/nebi-projects by default", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv(ProjectsDirEnv, "")
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
