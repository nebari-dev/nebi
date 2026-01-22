package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
)

func TestHandleGlobalPull_NewWorkspace(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	outputDir, err := handleGlobalPull(store, "uuid-123", "data-science", "v1.0")
	if err != nil {
		t.Fatalf("handleGlobalPull() error = %v", err)
	}

	expected := store.GlobalWorkspacePath("uuid-123", "v1.0")
	if outputDir != expected {
		t.Errorf("outputDir = %q, want %q", outputDir, expected)
	}
}

func TestHandleGlobalPull_ExistingBlocked(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Add existing global entry
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      store.GlobalWorkspacePath("uuid-123", "v1.0"),
		IsGlobal:  true,
		PulledAt:  time.Now(),
	})

	// Should be blocked without --force
	pullForce = false
	_, err := handleGlobalPull(store, "uuid-123", "data-science", "v1.0")
	if err == nil {
		t.Fatal("handleGlobalPull() should return error for existing workspace without --force")
	}
}

func TestHandleGlobalPull_ExistingForced(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Add existing global entry
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      store.GlobalWorkspacePath("uuid-123", "v1.0"),
		IsGlobal:  true,
		PulledAt:  time.Now(),
	})

	// Should succeed with --force
	pullForce = true
	defer func() { pullForce = false }()

	outputDir, err := handleGlobalPull(store, "uuid-123", "data-science", "v1.0")
	if err != nil {
		t.Fatalf("handleGlobalPull() with --force error = %v", err)
	}

	expected := store.GlobalWorkspacePath("uuid-123", "v1.0")
	if outputDir != expected {
		t.Errorf("outputDir = %q, want %q", outputDir, expected)
	}
}

func TestHandleGlobalPull_DifferentTag(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Add existing global entry for v1.0
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      store.GlobalWorkspacePath("uuid-123", "v1.0"),
		IsGlobal:  true,
		PulledAt:  time.Now(),
	})

	// Pull v2.0 should succeed (different tag = separate directory)
	pullForce = false
	outputDir, err := handleGlobalPull(store, "uuid-123", "data-science", "v2.0")
	if err != nil {
		t.Fatalf("handleGlobalPull() for different tag error = %v", err)
	}

	expected := store.GlobalWorkspacePath("uuid-123", "v2.0")
	if outputDir != expected {
		t.Errorf("outputDir = %q, want %q", outputDir, expected)
	}
}

func TestHandleDirectoryPull_NewDirectory(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	pullOutput = filepath.Join(dir, "output")
	pullForce = false
	pullYes = false

	outputDir, err := handleDirectoryPull(store, "data-science", "v1.0")
	if err != nil {
		t.Fatalf("handleDirectoryPull() error = %v", err)
	}

	if outputDir != pullOutput {
		t.Errorf("outputDir = %q, want %q", outputDir, pullOutput)
	}
}

func TestHandleDirectoryPull_SameWorkspaceTag(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	outputPath := filepath.Join(dir, "output")
	os.MkdirAll(outputPath, 0755)

	// Add existing entry for same workspace:tag
	absPath, _ := filepath.Abs(outputPath)
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      absPath,
		PulledAt:  time.Now(),
	})

	pullOutput = outputPath
	pullForce = false
	pullYes = false

	// Same workspace:tag to same dir should succeed (re-pull, no prompt)
	outputDir, err := handleDirectoryPull(store, "data-science", "v1.0")
	if err != nil {
		t.Fatalf("handleDirectoryPull() error = %v", err)
	}

	if outputDir != pullOutput {
		t.Errorf("outputDir = %q, want %q", outputDir, pullOutput)
	}
}

func TestHandleDirectoryPull_DifferentTagWithForce(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	outputPath := filepath.Join(dir, "output")
	os.MkdirAll(outputPath, 0755)

	// Add existing entry for different tag
	absPath, _ := filepath.Abs(outputPath)
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      absPath,
		PulledAt:  time.Now(),
	})

	pullOutput = outputPath
	pullForce = true
	defer func() { pullForce = false }()

	// Different tag with --force should succeed
	outputDir, err := handleDirectoryPull(store, "data-science", "v2.0")
	if err != nil {
		t.Fatalf("handleDirectoryPull() with --force error = %v", err)
	}

	if outputDir != pullOutput {
		t.Errorf("outputDir = %q, want %q", outputDir, pullOutput)
	}
}

func TestPullIntegration_WritesNebiFile(t *testing.T) {
	dir := t.TempDir()

	// Simulate what runPull does after fetching content
	pixiTomlContent := []byte("[workspace]\nname = \"test\"\n")
	pixiLockContent := []byte("version: 1\npackages: []\n")

	// Write files
	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiTomlContent, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLockContent, 0644)

	// Compute digests
	tomlDigest := nebifile.ComputeDigest(pixiTomlContent)
	lockDigest := nebifile.ComputeDigest(pixiLockContent)

	// Write .nebi file
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://nebi.example.com",
		1, "sha256:manifest123",
		tomlDigest, int64(len(pixiTomlContent)),
		lockDigest, int64(len(pixiLockContent)),
	)
	if err := nebifile.Write(dir, nf); err != nil {
		t.Fatalf("nebifile.Write() error = %v", err)
	}

	// Verify .nebi file was created
	if !nebifile.Exists(dir) {
		t.Fatal(".nebi file should exist")
	}

	// Read it back
	loaded, err := nebifile.Read(dir)
	if err != nil {
		t.Fatalf("nebifile.Read() error = %v", err)
	}

	if loaded.Origin.Workspace != "test-workspace" {
		t.Errorf("Workspace = %q, want %q", loaded.Origin.Workspace, "test-workspace")
	}
	if loaded.Origin.Tag != "v1.0" {
		t.Errorf("Tag = %q, want %q", loaded.Origin.Tag, "v1.0")
	}
	if loaded.Origin.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", loaded.Origin.ServerURL, "https://nebi.example.com")
	}
	if loaded.GetLayerDigest("pixi.toml") != tomlDigest {
		t.Errorf("pixi.toml digest mismatch")
	}
	if loaded.GetLayerDigest("pixi.lock") != lockDigest {
		t.Errorf("pixi.lock digest mismatch")
	}
}

func TestPullIntegration_UpdatesIndex(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Simulate adding an entry (as runPull does)
	entry := localindex.WorkspaceEntry{
		Workspace:       "test-workspace",
		Tag:             "v1.0",
		ServerURL:       "https://nebi.example.com",
		ServerVersionID: 1,
		Path:            filepath.Join(dir, "workspace"),
		IsGlobal:        false,
		PulledAt:        time.Now(),
		ManifestDigest:  "sha256:manifest123",
		Layers: map[string]string{
			"pixi.toml": "sha256:toml456",
			"pixi.lock": "sha256:lock789",
		},
	}

	if err := store.AddEntry(entry); err != nil {
		t.Fatalf("AddEntry() error = %v", err)
	}

	// Verify entry is in index
	found, err := store.FindByPath(entry.Path)
	if err != nil {
		t.Fatalf("FindByPath() error = %v", err)
	}
	if found == nil {
		t.Fatal("Entry should be found in index")
	}
	if found.Workspace != "test-workspace" {
		t.Errorf("Workspace = %q, want %q", found.Workspace, "test-workspace")
	}
	if found.ManifestDigest != "sha256:manifest123" {
		t.Errorf("ManifestDigest = %q, want %q", found.ManifestDigest, "sha256:manifest123")
	}
}

func TestPullIntegration_GlobalWithAlias(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Simulate global pull with alias
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	tag := "v1.0"

	entry := localindex.WorkspaceEntry{
		Workspace:       "data-science",
		Tag:             tag,
		ServerURL:       "https://nebi.example.com",
		ServerVersionID: 42,
		Path:            store.GlobalWorkspacePath(uuid, tag),
		IsGlobal:        true,
		PulledAt:        time.Now(),
		ManifestDigest:  "sha256:abc123",
		Layers: map[string]string{
			"pixi.toml": "sha256:111",
			"pixi.lock": "sha256:222",
		},
	}

	if err := store.AddEntry(entry); err != nil {
		t.Fatalf("AddEntry() error = %v", err)
	}

	// Set alias
	alias := localindex.Alias{UUID: uuid, Tag: tag}
	if err := store.SetAlias("ds-stable", alias); err != nil {
		t.Fatalf("SetAlias() error = %v", err)
	}

	// Verify alias resolves
	got, err := store.GetAlias("ds-stable")
	if err != nil {
		t.Fatalf("GetAlias() error = %v", err)
	}
	if got == nil {
		t.Fatal("Alias should exist")
	}
	if got.UUID != uuid {
		t.Errorf("Alias UUID = %q, want %q", got.UUID, uuid)
	}
	if got.Tag != tag {
		t.Errorf("Alias Tag = %q, want %q", got.Tag, tag)
	}

	// Verify global entry is findable
	global, err := store.FindGlobal("data-science", "v1.0")
	if err != nil {
		t.Fatalf("FindGlobal() error = %v", err)
	}
	if global == nil {
		t.Fatal("Global entry should be found")
	}
	if !global.IsGlobal {
		t.Error("Entry should be global")
	}
}

func TestPullIntegration_DirectoryPullDuplicateAllowed(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	now := time.Now()

	// Pull to path A
	entryA := localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      filepath.Join(dir, "project-a"),
		PulledAt:  now,
	}
	store.AddEntry(entryA)

	// Pull same workspace:tag to path B (allowed for directory pulls)
	entryB := localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      filepath.Join(dir, "project-b"),
		PulledAt:  now.Add(time.Hour),
	}
	store.AddEntry(entryB)

	// Both should exist
	matches, err := store.FindByWorkspaceTag("data-science", "v1.0")
	if err != nil {
		t.Fatalf("FindByWorkspaceTag() error = %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("Expected 2 entries for same workspace:tag, got %d", len(matches))
	}
}
