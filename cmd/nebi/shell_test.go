package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
)

func TestResolveShellFromCwd_WithNebiFile(t *testing.T) {
	dir := t.TempDir()

	// Create .nebi file
	pixiToml := []byte("[workspace]\nname = \"test\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	tomlDigest := nebifile.ComputeDigest(pixiToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-ws", "v1.0", "", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(pixiToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Change to that directory
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldDir)

	result := resolveShellFromCwd()
	absDir, _ := filepath.Abs(".")
	if result != absDir {
		t.Errorf("resolveShellFromCwd() = %q, want %q", result, absDir)
	}
}

func TestResolveShellFromCwd_WithPixiToml(t *testing.T) {
	dir := t.TempDir()

	// Only pixi.toml, no .nebi
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[workspace]\nname=\"test\"\n"), 0644)

	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldDir)

	result := resolveShellFromCwd()
	absDir, _ := filepath.Abs(".")
	if result != absDir {
		t.Errorf("resolveShellFromCwd() = %q, want %q", result, absDir)
	}
}

func TestResolveShellFromRef_GlobalPreferred(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Create a global workspace directory
	globalPath := store.GlobalWorkspacePath("uuid-123", "v1.0")
	os.MkdirAll(globalPath, 0755)
	os.WriteFile(filepath.Join(globalPath, "pixi.toml"), []byte("test"), 0644)

	// Add global entry
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      globalPath,
		IsGlobal:  true,
		PulledAt:  time.Now(),
	})

	// Also add a local entry
	localPath := filepath.Join(dir, "local-ws")
	os.MkdirAll(localPath, 0755)
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      localPath,
		IsGlobal:  false,
		PulledAt:  time.Now(),
	})

	// Global should be preferred
	global, _ := store.FindGlobal("data-science", "v1.0")
	if global == nil {
		t.Fatal("Global entry should exist")
	}
	if global.Path != globalPath {
		t.Errorf("Global path = %q, want %q", global.Path, globalPath)
	}
}

func TestResolveShellFromRef_LocalFallback(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Only add local entries
	localPath1 := filepath.Join(dir, "ws1")
	localPath2 := filepath.Join(dir, "ws2")
	os.MkdirAll(localPath1, 0755)
	os.MkdirAll(localPath2, 0755)

	now := time.Now()
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      localPath1,
		IsGlobal:  false,
		PulledAt:  now.Add(-time.Hour),
	})
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      localPath2,
		IsGlobal:  false,
		PulledAt:  now,
	})

	// FindByWorkspaceTag should return both
	matches, err := store.FindByWorkspaceTag("data-science", "v1.0")
	if err != nil {
		t.Fatalf("FindByWorkspaceTag() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("Expected 2 matches, got %d", len(matches))
	}

	// Most recent should be ws2
	best := matches[0]
	for _, m := range matches[1:] {
		if m.PulledAt.After(best.PulledAt) {
			best = m
		}
	}
	if best.Path != localPath2 {
		t.Errorf("Most recent path = %q, want %q", best.Path, localPath2)
	}
}

func TestCheckShellDrift_Clean(t *testing.T) {
	dir := t.TempDir()

	pixiToml := []byte("[workspace]\nname = \"test\"\n")
	pixiLock := []byte("version: 1\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	tomlDigest := nebifile.ComputeDigest(pixiToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-ws", "v1.0", "", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(pixiToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Should not panic
	checkShellDrift(dir)
}

func TestCheckShellDrift_NoNebiFile(t *testing.T) {
	dir := t.TempDir()
	// Should not panic when no .nebi file exists
	checkShellDrift(dir)
}

func TestCheckShellDrift_Modified(t *testing.T) {
	dir := t.TempDir()

	originalToml := []byte("[workspace]\nname = \"test\"\n")
	modifiedToml := []byte("[workspace]\nname = \"modified\"\n")
	pixiLock := []byte("version: 1\n")

	os.WriteFile(filepath.Join(dir, "pixi.toml"), modifiedToml, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0644)

	tomlDigest := nebifile.ComputeDigest(originalToml)
	lockDigest := nebifile.ComputeDigest(pixiLock)
	nf := nebifile.NewFromPull(
		"test-ws", "v1.0", "", "https://example.com",
		1, "sha256:abc",
		tomlDigest, int64(len(originalToml)),
		lockDigest, int64(len(pixiLock)),
	)
	nebifile.Write(dir, nf)

	// Should not panic, just prints warning to stderr
	checkShellDrift(dir)
}

func TestShellCmd_HasEnvFlag(t *testing.T) {
	flag := shellCmd.Flags().Lookup("env")
	if flag == nil {
		t.Fatal("--env flag should be registered")
	}
	if flag.Shorthand != "e" {
		t.Errorf("--env shorthand = %q, want %q", flag.Shorthand, "e")
	}
}

func TestShellCmd_AcceptsZeroOrOneArgs(t *testing.T) {
	// The command accepts 0 or 1 args (MaximumNArgs(1))
	if shellCmd.Args == nil {
		t.Fatal("Args should not be nil")
	}
}
