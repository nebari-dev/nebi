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

	expected := store.GlobalRepoPath("uuid-123", "v1.0")
	if outputDir != expected {
		t.Errorf("outputDir = %q, want %q", outputDir, expected)
	}
}

func TestHandleGlobalPull_ExistingBlocked(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Add existing global entry
	store.AddEntry(localindex.Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		Path:        store.GlobalRepoPath("uuid-123", "v1.0"),
		PulledAt:    time.Now(),
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
	store.AddEntry(localindex.Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		Path:        store.GlobalRepoPath("uuid-123", "v1.0"),
		PulledAt:    time.Now(),
	})

	// Should succeed with --force
	pullForce = true
	defer func() { pullForce = false }()

	outputDir, err := handleGlobalPull(store, "uuid-123", "data-science", "v1.0")
	if err != nil {
		t.Fatalf("handleGlobalPull() with --force error = %v", err)
	}

	expected := store.GlobalRepoPath("uuid-123", "v1.0")
	if outputDir != expected {
		t.Errorf("outputDir = %q, want %q", outputDir, expected)
	}
}

func TestHandleGlobalPull_DifferentTag(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Add existing global entry for v1.0
	store.AddEntry(localindex.Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		Path:        store.GlobalRepoPath("uuid-123", "v1.0"),
		PulledAt:    time.Now(),
	})

	// Pull v2.0 should succeed (different tag = separate directory)
	pullForce = false
	outputDir, err := handleGlobalPull(store, "uuid-123", "data-science", "v2.0")
	if err != nil {
		t.Fatalf("handleGlobalPull() for different tag error = %v", err)
	}

	expected := store.GlobalRepoPath("uuid-123", "v2.0")
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
	store.AddEntry(localindex.Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		Path:        absPath,
		PulledAt:    time.Now(),
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
	store.AddEntry(localindex.Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		Path:        absPath,
		PulledAt:    time.Now(),
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

	// Write .nebi file (using new simplified signature)
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "https://nebi.example.com", "", "1", "",
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

	if loaded.Origin.SpecName != "test-workspace" {
		t.Errorf("SpecName = %q, want %q", loaded.Origin.SpecName, "test-workspace")
	}
	if loaded.Origin.VersionName != "v1.0" {
		t.Errorf("VersionName = %q, want %q", loaded.Origin.VersionName, "v1.0")
	}
	if loaded.Origin.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", loaded.Origin.ServerURL, "https://nebi.example.com")
	}
}

func TestPullIntegration_UpdatesIndex(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Simulate adding an entry (as runPull does)
	entry := localindex.Entry{
		SpecName:    "test-workspace",
		VersionName: "v1.0",
		VersionID:   "1",
		ServerURL:   "https://nebi.example.com",
		Path:        filepath.Join(dir, "workspace"),
		PulledAt:    time.Now(),
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
	if found.SpecName != "test-workspace" {
		t.Errorf("SpecName = %q, want %q", found.SpecName, "test-workspace")
	}
	if found.VersionID != "1" {
		t.Errorf("VersionID = %q, want %q", found.VersionID, "1")
	}
}

