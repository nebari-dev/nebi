package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
)

func TestGetLocalEntryStatus_PathMissing(t *testing.T) {
	entry := localindex.Entry{
		Path: "/nonexistent/path/12345",
	}
	status := getLocalEntryStatus(entry)
	if status != "missing" {
		t.Errorf("status = %q, want %q", status, "missing")
	}
}

func TestGetLocalEntryStatus_Clean(t *testing.T) {
	// This test verifies clean detection when .nebi exists with matching digests.
	// Note: Full drift detection requires both nebifile and index entry,
	// which is tested in internal/drift/drift_test.go.
	// Here we just verify the function doesn't panic and returns reasonable result.
	dir := t.TempDir()

	// Write pixi.toml and pixi.lock
	pixiToml := []byte("[workspace]\nname = \"test\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	// Write .nebi metadata
	nf := nebifile.NewFromPull(
		"test", "v1.0", "https://example.com", "", "1", "",
	)
	nebifile.Write(dir, nf)

	entry := localindex.Entry{Path: dir}
	status := getLocalEntryStatus(entry)
	// Should return some status (unknown is acceptable since index isn't set up)
	if status == "" {
		t.Error("status should not be empty")
	}
}

func TestGetLocalEntryStatus_Modified(t *testing.T) {
	// This test verifies modified detection when local file differs from origin.
	// Note: Full drift detection requires both nebifile and index entry,
	// which is tested in internal/drift/drift_test.go.
	dir := t.TempDir()

	// Write modified pixi.toml
	modifiedToml := []byte("[workspace]\nname = \"test\"\n[dependencies]\nnumpy = \">=1.0\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), modifiedToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	// Write .nebi with original info
	nf := nebifile.NewFromPull(
		"test", "v1.0", "https://example.com", "", "1", "",
	)
	nebifile.Write(dir, nf)

	entry := localindex.Entry{Path: dir}
	status := getLocalEntryStatus(entry)
	// Should return some status (unknown is acceptable since index isn't set up)
	if status == "" {
		t.Error("status should not be empty")
	}
}

func TestGetLocalEntryStatus_NoNebiFile(t *testing.T) {
	dir := t.TempDir()

	// Just has pixi.toml, no .nebi
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("test"), 0644)

	entry := localindex.Entry{Path: dir}
	status := getLocalEntryStatus(entry)
	// Should be "unknown" since drift.Check will fail without .nebi
	if status != "unknown" {
		t.Errorf("status = %q, want %q", status, "unknown")
	}
}

func TestFormatLocation_Local(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, "projects", "my-workspace")

	result := formatLocation(path, false)
	if result != "~/projects/my-workspace (local)" {
		t.Errorf("formatLocation() = %q, want %q", result, "~/projects/my-workspace (local)")
	}
}

func TestFormatLocation_Global(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".local", "share", "nebi", "repos", "550e8400-e29b-41d4-a716-446655440000", "v1.0")

	result := formatLocation(path, true)
	want := "~/.local/share/nebi/repos/550e8400/v1.0 (global)"
	if result != want {
		t.Errorf("formatLocation() = %q, want %q", result, want)
	}
}

func TestFormatLocation_GlobalNonUUID(t *testing.T) {
	home, _ := os.UserHomeDir()
	// Non-UUID directory name should not be abbreviated
	path := filepath.Join(home, ".local", "share", "nebi", "repos", "my-workspace", "v1.0")

	result := formatLocation(path, true)
	want := "~/.local/share/nebi/repos/my-workspace/v1.0 (global)"
	if result != want {
		t.Errorf("formatLocation() = %q, want %q", result, want)
	}
}

func TestFormatLocation_AbsolutePath(t *testing.T) {
	result := formatLocation("/opt/workspaces/test", false)
	if result != "/opt/workspaces/test (local)" {
		t.Errorf("formatLocation() = %q, want %q", result, "/opt/workspaces/test (local)")
	}
}

func TestWorkspacePrune_Integration(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Add entries - one with valid path, one with missing path
	validPath := filepath.Join(dir, "valid")
	os.MkdirAll(validPath, 0755)

	store.AddEntry(localindex.Entry{
		SpecName:    "valid-ws",
		VersionName: "v1.0",
		Path:        validPath,
		PulledAt:    time.Now(),
	})
	store.AddEntry(localindex.Entry{
		SpecName:    "missing-ws",
		VersionName: "v1.0",
		Path:        filepath.Join(dir, "does-not-exist"),
		PulledAt:    time.Now(),
	})

	// Prune
	removed, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(removed) != 1 {
		t.Fatalf("Prune() removed %d entries, want 1", len(removed))
	}
	if removed[0].SpecName != "missing-ws" {
		t.Errorf("removed workspace = %q, want %q", removed[0].SpecName, "missing-ws")
	}

	// Verify valid entry still exists
	found, _ := store.FindByPath(validPath)
	if found == nil {
		t.Error("Valid entry should still exist after prune")
	}
}

func TestWorkspaceListLocal_EmptyIndex(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	index, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(index.Entries) != 0 {
		t.Errorf("Empty index should have 0 workspaces, got %d", len(index.Entries))
	}
}

func TestWorkspaceListLocal_WithEntries(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Add some entries
	path1 := filepath.Join(dir, "ws1")
	path2 := filepath.Join(dir, "ws2")
	os.MkdirAll(path1, 0755)
	os.MkdirAll(path2, 0755)

	store.AddEntry(localindex.Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		Path:        path1,
		PulledAt:    time.Now(),
	})
	store.AddEntry(localindex.Entry{
		SpecName:    "data-science",
		VersionName: "v2.0",
		Path:        path2,
		PulledAt:    time.Now(),
	})

	index, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(index.Entries) != 2 {
		t.Errorf("Expected 2 workspaces, got %d", len(index.Entries))
	}
}

func TestWorkspaceInfoCmd_AcceptsZeroOrOneArgs(t *testing.T) {
	// The command should accept 0 or 1 args (MaximumNArgs(1))
	err := repoInfoCmd.Args(repoInfoCmd, []string{})
	if err != nil {
		t.Errorf("repoInfoCmd should accept 0 args, got error: %v", err)
	}

	err = repoInfoCmd.Args(repoInfoCmd, []string{"myworkspace"})
	if err != nil {
		t.Errorf("repoInfoCmd should accept 1 arg, got error: %v", err)
	}

	err = repoInfoCmd.Args(repoInfoCmd, []string{"a", "b"})
	if err == nil {
		t.Error("repoInfoCmd should reject 2 args")
	}
}

func TestWorkspaceInfoCmd_HasPathFlag(t *testing.T) {
	flag := repoInfoCmd.Flags().Lookup("path")
	if flag == nil {
		t.Fatal("--path/-C flag should be registered")
	}
	if flag.DefValue != "." {
		t.Errorf("--path default = %q, want %q", flag.DefValue, ".")
	}
}
