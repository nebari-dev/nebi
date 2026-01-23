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

func TestPullCmd_HasGlobalShortFlag(t *testing.T) {
	flag := pullCmd.Flags().Lookup("global")
	if flag == nil {
		t.Fatal("--global flag should be registered")
	}
	if flag.Shorthand != "g" {
		t.Errorf("--global shorthand = %q, want %q", flag.Shorthand, "g")
	}
}

func TestPullCmd_HasInstallFlag(t *testing.T) {
	flag := pullCmd.Flags().Lookup("install")
	if flag == nil {
		t.Fatal("--install flag should be registered")
	}
	if flag.Shorthand != "i" {
		t.Errorf("--install shorthand = %q, want %q", flag.Shorthand, "i")
	}
}

func TestPullCmd_FlagShorthandsDoNotConflict(t *testing.T) {
	// Verify all known pull command short flags are unique and correctly mapped
	shortFlags := map[string]string{
		"o": "output",
		"g": "global",
		"i": "install",
	}

	for short, long := range shortFlags {
		flag := pullCmd.Flags().ShorthandLookup(short)
		if flag == nil {
			t.Errorf("Short flag -%s should exist (for --%s)", short, long)
			continue
		}
		if flag.Name != long {
			t.Errorf("Short flag -%s maps to --%s, want --%s", short, flag.Name, long)
		}
	}
}

func TestCheckAlreadyUpToDate_NoNebiFile(t *testing.T) {
	dir := t.TempDir()
	// No .nebi file → should not skip
	if checkAlreadyUpToDate(dir, "ws", "v1.0", "sha256:abc") {
		t.Error("should not skip when .nebi file is missing")
	}
}

func TestCheckAlreadyUpToDate_DifferentWorkspace(t *testing.T) {
	dir := t.TempDir()
	content := []byte("test content")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), content, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), content, 0644)

	digest := nebifile.ComputeDigest(content)
	nf := nebifile.NewFromPull("other-ws", "v1.0", "", "http://localhost",
		1, "sha256:abc", digest, int64(len(content)), digest, int64(len(content)))
	nebifile.Write(dir, nf)

	if checkAlreadyUpToDate(dir, "my-ws", "v1.0", "sha256:abc") {
		t.Error("should not skip when workspace name differs")
	}
}

func TestCheckAlreadyUpToDate_DifferentTag(t *testing.T) {
	dir := t.TempDir()
	content := []byte("test content")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), content, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), content, 0644)

	digest := nebifile.ComputeDigest(content)
	nf := nebifile.NewFromPull("ws", "v1.0", "", "http://localhost",
		1, "sha256:abc", digest, int64(len(content)), digest, int64(len(content)))
	nebifile.Write(dir, nf)

	if checkAlreadyUpToDate(dir, "ws", "v2.0", "sha256:abc") {
		t.Error("should not skip when tag differs")
	}
}

func TestCheckAlreadyUpToDate_CleanAndSameDigest(t *testing.T) {
	dir := t.TempDir()
	tomlContent := []byte("[workspace]\nname = \"test\"\n")
	lockContent := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), tomlContent, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), lockContent, 0644)

	tomlDigest := nebifile.ComputeDigest(tomlContent)
	lockDigest := nebifile.ComputeDigest(lockContent)
	nf := nebifile.NewFromPull("ws", "v1.0", "", "http://localhost",
		1, "sha256:abc", tomlDigest, int64(len(tomlContent)), lockDigest, int64(len(lockContent)))
	nebifile.Write(dir, nf)

	if !checkAlreadyUpToDate(dir, "ws", "v1.0", "sha256:abc") {
		t.Error("should skip when workspace is clean and digest matches")
	}
}

func TestCheckAlreadyUpToDate_CleanButDigestDiffers(t *testing.T) {
	dir := t.TempDir()
	tomlContent := []byte("[workspace]\nname = \"test\"\n")
	lockContent := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), tomlContent, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), lockContent, 0644)

	tomlDigest := nebifile.ComputeDigest(tomlContent)
	lockDigest := nebifile.ComputeDigest(lockContent)
	nf := nebifile.NewFromPull("ws", "v1.0", "", "http://localhost",
		1, "sha256:abc", tomlDigest, int64(len(tomlContent)), lockDigest, int64(len(lockContent)))
	nebifile.Write(dir, nf)

	// Remote digest differs — tag has been updated
	if checkAlreadyUpToDate(dir, "ws", "v1.0", "sha256:xyz-new") {
		t.Error("should not skip when remote digest differs (tag updated)")
	}
}

func TestCheckAlreadyUpToDate_ModifiedFilesWithYes(t *testing.T) {
	dir := t.TempDir()
	tomlContent := []byte("[workspace]\nname = \"test\"\n")
	lockContent := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), tomlContent, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), lockContent, 0644)

	tomlDigest := nebifile.ComputeDigest(tomlContent)
	lockDigest := nebifile.ComputeDigest(lockContent)
	nf := nebifile.NewFromPull("ws", "v1.0", "", "http://localhost",
		1, "sha256:abc", tomlDigest, int64(len(tomlContent)), lockDigest, int64(len(lockContent)))
	nebifile.Write(dir, nf)

	// Modify a file after writing .nebi
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("modified!"), 0644)

	// With --yes, modified files should NOT skip (proceed with re-pull)
	pullYes = true
	defer func() { pullYes = false }()

	if checkAlreadyUpToDate(dir, "ws", "v1.0", "sha256:abc") {
		t.Error("should not skip when files are modified and --yes is set")
	}
}

func TestCheckAlreadyUpToDate_EmptyRemoteDigest(t *testing.T) {
	dir := t.TempDir()
	tomlContent := []byte("[workspace]\nname = \"test\"\n")
	lockContent := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), tomlContent, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), lockContent, 0644)

	tomlDigest := nebifile.ComputeDigest(tomlContent)
	lockDigest := nebifile.ComputeDigest(lockContent)
	nf := nebifile.NewFromPull("ws", "v1.0", "", "http://localhost",
		1, "sha256:abc", tomlDigest, int64(len(tomlContent)), lockDigest, int64(len(lockContent)))
	nebifile.Write(dir, nf)

	// Empty remote digest (e.g., pulling "latest" with no digest info) — skip digest check
	if !checkAlreadyUpToDate(dir, "ws", "v1.0", "") {
		t.Error("should skip when files are clean and remote digest is empty (no digest comparison)")
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
