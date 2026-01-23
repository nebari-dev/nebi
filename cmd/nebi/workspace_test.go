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
	path := filepath.Join(home, ".local", "share", "nebi", "workspaces", "550e8400-e29b-41d4-a716-446655440000", "v1.0")

	result := formatLocation(path, true)
	want := "~/.local/share/nebi/workspaces/550e8400/v1.0 (global)"
	if result != want {
		t.Errorf("formatLocation() = %q, want %q", result, want)
	}
}

func TestFormatLocation_GlobalNonUUID(t *testing.T) {
	home, _ := os.UserHomeDir()
	// Non-UUID directory name should not be abbreviated
	path := filepath.Join(home, ".local", "share", "nebi", "workspaces", "my-workspace", "v1.0")

	result := formatLocation(path, true)
	want := "~/.local/share/nebi/workspaces/my-workspace/v1.0 (global)"
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

func TestWorkspaceInfoCmd_AcceptsZeroOrOneArgs(t *testing.T) {
	// The command should accept 0 or 1 args (MaximumNArgs(1))
	err := workspaceInfoCmd.Args(workspaceInfoCmd, []string{})
	if err != nil {
		t.Errorf("workspaceInfoCmd should accept 0 args, got error: %v", err)
	}

	err = workspaceInfoCmd.Args(workspaceInfoCmd, []string{"myworkspace"})
	if err != nil {
		t.Errorf("workspaceInfoCmd should accept 1 arg, got error: %v", err)
	}

	err = workspaceInfoCmd.Args(workspaceInfoCmd, []string{"a", "b"})
	if err == nil {
		t.Error("workspaceInfoCmd should reject 2 args")
	}
}

func TestWorkspaceInfoCmd_HasPathFlag(t *testing.T) {
	flag := workspaceInfoCmd.Flags().Lookup("path")
	if flag == nil {
		t.Fatal("--path/-C flag should be registered")
	}
	if flag.DefValue != "." {
		t.Errorf("--path default = %q, want %q", flag.DefValue, ".")
	}
	if flag.Shorthand != "C" {
		t.Errorf("--path shorthand = %q, want %q", flag.Shorthand, "C")
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

func TestWorkspaceListCmd_HasJSONFlag(t *testing.T) {
	flag := workspaceListCmd.Flags().Lookup("json")
	if flag == nil {
		t.Fatal("--json flag should be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--json default = %q, want %q", flag.DefValue, "false")
	}
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"ABCDEF00-1234-5678-9ABC-DEF012345678", true},
		{"not-a-uuid", false},
		{"550e8400e29b41d4a716446655440000", false},   // no hyphens
		{"550e8400-e29b-41d4-a716-44665544000", false}, // too short
		{"v1.0", false},
		{"", false},
		{"workspaces", false},
	}
	for _, tt := range tests {
		got := isUUID(tt.input)
		if got != tt.want {
			t.Errorf("isUUID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAbbreviateUUID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"~/.local/share/nebi/workspaces/550e8400-e29b-41d4-a716-446655440000/v1.0",
			"~/.local/share/nebi/workspaces/550e8400/v1.0",
		},
		{
			"~/.local/share/nebi/workspaces/my-workspace/v1.0",
			"~/.local/share/nebi/workspaces/my-workspace/v1.0",
		},
		{
			"/opt/nebi/workspaces/550e8400-e29b-41d4-a716-446655440000/v2.0",
			"/opt/nebi/workspaces/550e8400/v2.0",
		},
	}
	for _, tt := range tests {
		got := abbreviateUUID(tt.input)
		if got != tt.want {
			t.Errorf("abbreviateUUID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
