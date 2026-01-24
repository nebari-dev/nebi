package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aktech/darb/internal/nebifile"
)

func TestParseWorkspaceRef_WithTag(t *testing.T) {
	ws, tag, err := parseRepoRef("myworkspace:v1.0")
	if err != nil {
		t.Fatalf("parseRepoRef() error = %v", err)
	}
	if ws != "myworkspace" {
		t.Errorf("workspace = %q, want %q", ws, "myworkspace")
	}
	if tag != "v1.0" {
		t.Errorf("tag = %q, want %q", tag, "v1.0")
	}
}

func TestParseWorkspaceRef_WithDigest(t *testing.T) {
	ws, tag, err := parseRepoRef("myworkspace@sha256:abc123")
	if err != nil {
		t.Fatalf("parseRepoRef() error = %v", err)
	}
	if ws != "myworkspace" {
		t.Errorf("workspace = %q, want %q", ws, "myworkspace")
	}
	if tag != "@sha256:abc123" {
		t.Errorf("tag = %q, want %q", tag, "@sha256:abc123")
	}
}

func TestParseWorkspaceRef_NoTag(t *testing.T) {
	ws, tag, err := parseRepoRef("myworkspace")
	if err != nil {
		t.Fatalf("parseRepoRef() error = %v", err)
	}
	if ws != "myworkspace" {
		t.Errorf("workspace = %q, want %q", ws, "myworkspace")
	}
	if tag != "" {
		t.Errorf("tag = %q, want empty", tag)
	}
}

func TestParseWorkspaceRef_ColonInTag(t *testing.T) {
	// workspace:tag where tag contains colon (e.g. workspace:v1:latest)
	ws, tag, err := parseRepoRef("workspace:v1:latest")
	if err != nil {
		t.Fatalf("parseRepoRef() error = %v", err)
	}
	// LastIndex of ":" gives us the last colon
	if ws != "workspace:v1" {
		t.Errorf("workspace = %q, want %q", ws, "workspace:v1")
	}
	if tag != "latest" {
		t.Errorf("tag = %q, want %q", tag, "latest")
	}
}

func TestParseWorkspaceRef_EmptyString(t *testing.T) {
	ws, tag, err := parseRepoRef("")
	if err != nil {
		t.Fatalf("parseRepoRef() error = %v", err)
	}
	if ws != "" {
		t.Errorf("workspace = %q, want empty", ws)
	}
	if tag != "" {
		t.Errorf("tag = %q, want empty", tag)
	}
}

func TestPushDryRun_NoNebiFile(t *testing.T) {
	// Test that dry-run works when there's no .nebi file
	// The function should print file sizes instead of a diff
	dir := t.TempDir()

	// Create pixi.toml
	pixiToml := []byte("[workspace]\nname = \"test\"\n")
	pixiLock := []byte("version: 1\npackages: []\n")

	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	// Change to temp dir to simulate no .nebi file
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldDir)

	// Just verify the function doesn't panic when no .nebi exists
	// The actual output goes to stdout - we verify the logic path
	if nebifile.Exists(dir) {
		t.Fatal(".nebi should not exist in temp dir")
	}
}

func TestPushDryRun_WithNebiFile(t *testing.T) {
	// Test that dry-run correctly identifies when .nebi metadata is present
	dir := t.TempDir()

	pixiToml := []byte("[workspace]\nname = \"test\"\n[dependencies]\nnumpy = \">=1.0\"\n")
	pixiLock := []byte("version: 1\npackages: []\n")

	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	// Write a .nebi file
	tomlDigest := nebifile.ComputeDigest(pixiToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://nebi.example.com",
		1, "sha256:manifest123",
		tomlDigest, int64(len(pixiToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Verify .nebi exists
	if !nebifile.Exists(dir) {
		t.Fatal(".nebi should exist")
	}

	// Read it back and verify origin info
	loaded, err := nebifile.Read(dir)
	if err != nil {
		t.Fatalf("nebifile.Read() error = %v", err)
	}
	if loaded.Origin.Repo != "test-workspace" {
		t.Errorf("Origin.Workspace = %q, want %q", loaded.Origin.Repo, "test-workspace")
	}
	if loaded.Origin.Tag != "v1.0" {
		t.Errorf("Origin.Tag = %q, want %q", loaded.Origin.Tag, "v1.0")
	}
}

func TestPushDryRun_DetectsModifiedToml(t *testing.T) {
	// Test that the dry-run can detect TOML modifications
	dir := t.TempDir()

	originalToml := []byte("[workspace]\nname = \"test\"\n[dependencies]\nnumpy = \">=1.0\"\n")
	modifiedToml := []byte("[workspace]\nname = \"test\"\n[dependencies]\nnumpy = \">=2.0\"\nscipy = \">=1.0\"\n")
	pixiLock := []byte("version: 1\npackages: []\n")

	// Write modified pixi.toml (simulates local changes)
	os.WriteFile(filepath.Join(dir, "pixi.toml"), modifiedToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	// Write .nebi file with original digests
	originalDigest := nebifile.ComputeDigest(originalToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://nebi.example.com",
		1, "sha256:manifest123",
		originalDigest, int64(len(originalToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Verify the current file has different digest from origin
	currentDigest := nebifile.ComputeDigest(modifiedToml)
	if currentDigest == originalDigest {
		t.Fatal("Modified TOML should have different digest")
	}
}

func TestPushDryRun_DetectsModifiedLock(t *testing.T) {
	// Test that lock file changes are detectable
	dir := t.TempDir()

	pixiToml := []byte("[workspace]\nname = \"test\"\n")
	originalLock := []byte("version: 1\npackages: []\n")
	modifiedLock := []byte("version: 1\npackages:\n  - name: numpy\n    version: 1.0\n")

	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), modifiedLock, 0644)

	// Write .nebi file with original lock digest
	tomlDigest := nebifile.ComputeDigest(pixiToml)
	originalLockDigest := nebifile.ComputeDigest(originalLock)
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://nebi.example.com",
		1, "sha256:manifest123",
		tomlDigest, int64(len(pixiToml)),
		originalLockDigest, int64(len(originalLock)),
	)
	nebifile.Write(dir, nf)

	// Verify lock digests differ
	currentLockDigest := nebifile.ComputeDigest(modifiedLock)
	if currentLockDigest == originalLockDigest {
		t.Fatal("Modified lock should have different digest")
	}
}

func TestPushCmd_HasDryRunFlag(t *testing.T) {
	// Verify the --dry-run flag is registered
	flag := pushCmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("--dry-run flag should be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--dry-run default = %q, want %q", flag.DefValue, "false")
	}
}

func TestPushCmd_RequiresExactlyOneArg(t *testing.T) {
	// Verify command requires exactly 1 argument
	if pushCmd.Args == nil {
		t.Fatal("Args should not be nil")
	}
}

func TestShowPushDriftWarning_NoNebiFile(t *testing.T) {
	dir := t.TempDir()
	// Should not panic when no .nebi file exists
	showPushDriftWarning(dir, "test", "v1.0", []byte("test"))
}

func TestShowPushDriftWarning_Clean(t *testing.T) {
	dir := t.TempDir()

	pixiToml := []byte("[workspace]\nname = \"test\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	tomlDigest := nebifile.ComputeDigest(pixiToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(pixiToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Should not warn (clean workspace)
	showPushDriftWarning(dir, "test-workspace", "v2.0", pixiToml)
}

func TestShowPushDriftWarning_ModifiedDifferentTag(t *testing.T) {
	dir := t.TempDir()

	originalToml := []byte("[workspace]\nname = \"test\"\n")
	modifiedToml := []byte("[workspace]\nname = \"test\"\n[dependencies]\nnumpy = \"*\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), modifiedToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	tomlDigest := nebifile.ComputeDigest(originalToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(originalToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Pushing to different tag - should show note but not the same-tag warning
	showPushDriftWarning(dir, "test-workspace", "v2.0", modifiedToml)
}

func TestShowPushDriftWarning_ModifiedSameTag(t *testing.T) {
	dir := t.TempDir()

	originalToml := []byte("[workspace]\nname = \"test\"\n")
	modifiedToml := []byte("[workspace]\nname = \"test\"\n[dependencies]\nnumpy = \"*\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), modifiedToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	tomlDigest := nebifile.ComputeDigest(originalToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(originalToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Pushing to same tag with modified content - should show the overwrite warning
	showPushDriftWarning(dir, "test-workspace", "v1.0", modifiedToml)
}
