package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
)

func TestRenameProject(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p, _ := h.mustCreate(specRequest("my-analysis"))
	h.mustCreate(specRequest("other"))

	manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
	before := readFile(t, manifestPath)
	beforeInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	got, err := h.svc.RenameProject(ctx, p.ID, "sales-q3")
	if err != nil {
		t.Fatalf("RenameProject: %v", err)
	}
	if got.Name != "sales-q3" || got.Path != p.Path || got.ID != p.ID {
		t.Fatalf("unexpected result %+v", got)
	}
	if d := h.detail(p.ID); d.Name != "sales-q3" || d.Path != p.Path {
		t.Fatalf("row not renamed: %+v", d.ProjectSummary)
	}

	// Never touches disk: same directory, same pixi.toml bytes and mtime,
	// [workspace].name unchanged.
	if !exists(p.Path) || exists(filepath.Join(filepath.Dir(p.Path), "sales-q3")) {
		t.Fatal("the project directory must not be renamed")
	}
	afterInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, manifestPath) != before || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatal("pixi.toml must not change on rename")
	}
	if m, _ := artifact.ParseManifest(before); m.Name != "my-analysis" {
		t.Fatalf("[workspace].name should still be the old name, got %q", m.Name)
	}

	if _, err := h.svc.RenameProject(ctx, p.ID, "sales-q3"); err != nil {
		t.Fatalf("renaming to its own name must work: %v", err)
	}
	if _, err := h.svc.RenameProject(ctx, p.ID, "Sales Q3"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("want ErrInvalidName, got %v", err)
	}
	if _, err := h.svc.RenameProject(ctx, p.ID, "other"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("want ErrNameTaken, got %v", err)
	}
	if _, err := h.svc.RenameProject(ctx, uuid.New(), "free-name"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if d := h.detail(p.ID); d.Name != "sales-q3" {
		t.Fatalf("a rejected rename must not change the name, got %q", d.Name)
	}
}

func TestRenameLegacyNameIgnoresItsOwnCase(t *testing.T) {
	h := newHarness(t)
	legacy := &Project{Name: "Demo", Path: filepath.Join(t.TempDir(), "demo")}
	h.insertRow(legacy)
	// "demo" only collides with the project's own legacy name "Demo".
	if _, err := h.svc.RenameProject(context.Background(), legacy.ID, "demo"); err != nil {
		t.Fatalf("RenameProject: %v", err)
	}
	if d := h.detail(legacy.ID); d.Name != "demo" {
		t.Fatalf("got %q", d.Name)
	}
}
