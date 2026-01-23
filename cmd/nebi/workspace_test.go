package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
)

func TestGetLocalEntryStatus_PathMissing(t *testing.T) {
	entry := localindex.WorkspaceEntry{
		Path: "/nonexistent/path/12345",
	}
	status := getLocalEntryStatus(entry)
	if status != "missing" {
		t.Errorf("status = %q, want %q", status, "missing")
	}
}

func TestGetLocalEntryStatus_Clean(t *testing.T) {
	dir := t.TempDir()

	// Write pixi.toml and pixi.lock
	pixiToml := []byte("[workspace]\nname = \"test\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	// Write matching .nebi metadata
	tomlDigest := nebifile.ComputeDigest(pixiToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test", "v1.0", "", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(pixiToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	entry := localindex.WorkspaceEntry{Path: dir}
	status := getLocalEntryStatus(entry)
	if status != string(drift.StatusClean) {
		t.Errorf("status = %q, want %q", status, drift.StatusClean)
	}
}

func TestGetLocalEntryStatus_Modified(t *testing.T) {
	dir := t.TempDir()

	// Write pixi.toml with different content than metadata
	originalToml := []byte("[workspace]\nname = \"test\"\n")
	modifiedToml := []byte("[workspace]\nname = \"test\"\n[dependencies]\nnumpy = \">=1.0\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), modifiedToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	// Write .nebi with original digest
	tomlDigest := nebifile.ComputeDigest(originalToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test", "v1.0", "", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(originalToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	entry := localindex.WorkspaceEntry{Path: dir}
	status := getLocalEntryStatus(entry)
	if status != string(drift.StatusModified) {
		t.Errorf("status = %q, want %q", status, drift.StatusModified)
	}
}

func TestGetLocalEntryStatus_NoNebiFile(t *testing.T) {
	dir := t.TempDir()

	// Just has pixi.toml, no .nebi
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("test"), 0644)

	entry := localindex.WorkspaceEntry{Path: dir}
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
	path := filepath.Join(home, ".local", "share", "nebi", "workspaces", "uuid", "v1.0")

	result := formatLocation(path, true)
	if result != "~/.local/share/nebi/... (global)" {
		t.Errorf("formatLocation() = %q, want %q", result, "~/.local/share/nebi/... (global)")
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

	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "valid-ws",
		Tag:       "v1.0",
		Path:      validPath,
		PulledAt:  time.Now(),
	})
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "missing-ws",
		Tag:       "v1.0",
		Path:      filepath.Join(dir, "does-not-exist"),
		PulledAt:  time.Now(),
	})

	// Prune
	removed, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(removed) != 1 {
		t.Fatalf("Prune() removed %d entries, want 1", len(removed))
	}
	if removed[0].Workspace != "missing-ws" {
		t.Errorf("removed workspace = %q, want %q", removed[0].Workspace, "missing-ws")
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
	if len(index.Workspaces) != 0 {
		t.Errorf("Empty index should have 0 workspaces, got %d", len(index.Workspaces))
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

	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      path1,
		IsGlobal:  false,
		PulledAt:  time.Now(),
	})
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v2.0",
		Path:      path2,
		IsGlobal:  true,
		PulledAt:  time.Now(),
	})

	index, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(index.Workspaces) != 2 {
		t.Errorf("Expected 2 workspaces, got %d", len(index.Workspaces))
	}
}

func TestWorkspacePruneCmd_HasNoArgs(t *testing.T) {
	if workspacePruneCmd.Args == nil {
		t.Fatal("Args should not be nil")
	}
}

func TestWorkspaceListCmd_HasLocalFlag(t *testing.T) {
	flag := workspaceListCmd.Flags().Lookup("local")
	if flag == nil {
		t.Fatal("--local flag should be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--local default = %q, want %q", flag.DefValue, "false")
	}
}
