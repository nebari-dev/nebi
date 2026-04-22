package oci

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// mkfile writes a zero-byte file at rel under root, creating parents.
func mkfile(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// asRelPaths extracts RelPath field from a walk result for comparison.
func asRelPaths(files []Asset) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.RelPath
	}
	sort.Strings(out)
	return out
}

func TestWalkBundle_CoreFilesAlwaysIncluded(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")

	files, err := walkBundle(root, bundleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	got := asRelPaths(files)
	want := []string{"pixi.lock", "pixi.toml"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_HardcodedDrops(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, ".git/HEAD")
	mkfile(t, root, ".git/config")
	mkfile(t, root, ".pixi/envs/default/conda-meta/x.json")
	mkfile(t, root, "README.md")

	files, err := walkBundle(root, bundleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"README.md", "pixi.lock", "pixi.toml"}
	if got := asRelPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_Gitignore(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, "src/main.go")
	mkfile(t, root, "build/out.o")
	mkfile(t, root, "trace.log")
	mkfile(t, root, "notes.txt")
	if err := os.WriteFile(
		filepath.Join(root, ".gitignore"),
		[]byte("build/\n*.log\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	files, err := walkBundle(root, bundleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "notes.txt", "pixi.lock", "pixi.toml", "src/main.go"}
	if got := asRelPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_ExcludeGlob(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, "secrets/api.key")
	mkfile(t, root, "secrets/cert.pem")
	mkfile(t, root, "src/app.go")

	files, err := walkBundle(root, bundleConfig{
		Exclude: []string{"secrets/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pixi.lock", "pixi.toml", "src/app.go"}
	if got := asRelPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_IncludeGlob(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, "src/app.go")
	mkfile(t, root, "docs/guide.md")
	mkfile(t, root, "scratch/tmp.py")

	files, err := walkBundle(root, bundleConfig{
		Include: []string{"src/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// With include filter active, only src/** and force-included core files.
	want := []string{"pixi.lock", "pixi.toml", "src/app.go"}
	if got := asRelPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_IncludeAndExcludeCombine(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, "src/app.go")
	mkfile(t, root, "src/debug.log")

	files, err := walkBundle(root, bundleConfig{
		Include: []string{"src/**"},
		Exclude: []string{"*.log"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pixi.lock", "pixi.toml", "src/app.go"}
	if got := asRelPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_ExcludeCannotDropCoreFiles(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, "README.md")

	files, err := walkBundle(root, bundleConfig{
		Exclude: []string{"pixi.toml", "pixi.lock", "README.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Core files force-included, README excluded.
	want := []string{"pixi.lock", "pixi.toml"}
	if got := asRelPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_GitignoreCannotDropCoreFiles(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	if err := os.WriteFile(
		filepath.Join(root, ".gitignore"),
		[]byte("pixi.toml\npixi.lock\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	files, err := walkBundle(root, bundleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "pixi.lock", "pixi.toml"}
	if got := asRelPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWalkBundle_SymlinksSkipped(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, "target.txt")
	if err := os.Symlink(filepath.Join(root, "target.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	files, err := walkBundle(root, bundleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.RelPath == "link.txt" {
			t.Fatalf("symlink should have been skipped: %+v", files)
		}
	}
}

func TestWalkBundle_DeterministicOrder(t *testing.T) {
	root := t.TempDir()
	mkfile(t, root, "pixi.toml")
	mkfile(t, root, "pixi.lock")
	mkfile(t, root, "z.txt")
	mkfile(t, root, "a.txt")
	mkfile(t, root, "m/x.txt")

	files, err := walkBundle(root, bundleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(files))
	for i, f := range files {
		got[i] = f.RelPath
	}
	want := []string{"a.txt", "m/x.txt", "pixi.lock", "pixi.toml", "z.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
