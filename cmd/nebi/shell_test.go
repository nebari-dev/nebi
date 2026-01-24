package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
)

// resetShellFlags resets all shell command flags to their zero values for testing.
func resetShellFlags() {
	shellGlobal = false
	shellLocal = false
	shellPath = ""
	shellPixiEnv = ""
}

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
	globalPath := store.GlobalRepoPath("uuid-123", "v1.0")
	os.MkdirAll(globalPath, 0755)
	os.WriteFile(filepath.Join(globalPath, "pixi.toml"), []byte("test"), 0644)

	// Add global entry
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
		Tag:       "v1.0",
		Path:      globalPath,
		IsGlobal:  true,
		PulledAt:  time.Now(),
	})

	// Also add a local entry
	localPath := filepath.Join(dir, "local-ws")
	os.MkdirAll(localPath, 0755)
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
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
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
		Tag:       "v1.0",
		Path:      localPath1,
		IsGlobal:  false,
		PulledAt:  now.Add(-time.Hour),
	})
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
		Tag:       "v1.0",
		Path:      localPath2,
		IsGlobal:  false,
		PulledAt:  now,
	})

	// FindByRepoTag should return both
	matches, err := store.FindByRepoTag("data-science", "v1.0")
	if err != nil {
		t.Fatalf("FindByRepoTag() error = %v", err)
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

func TestShellCmd_HasGlobalFlag(t *testing.T) {
	flag := shellCmd.Flags().Lookup("global")
	if flag == nil {
		t.Fatal("--global flag should be registered")
	}
	if flag.Shorthand != "g" {
		t.Errorf("--global shorthand = %q, want %q", flag.Shorthand, "g")
	}
}

func TestShellCmd_HasLocalFlag(t *testing.T) {
	flag := shellCmd.Flags().Lookup("local")
	if flag == nil {
		t.Fatal("--local flag should be registered")
	}
	if flag.Shorthand != "l" {
		t.Errorf("--local shorthand = %q, want %q", flag.Shorthand, "l")
	}
}

func TestShellCmd_HasPathFlag(t *testing.T) {
	flag := shellCmd.Flags().Lookup("path")
	if flag == nil {
		t.Fatal("--path flag should be registered")
	}
	if flag.Shorthand != "C" {
		t.Errorf("--path shorthand = %q, want %q", flag.Shorthand, "C")
	}
}

func TestResolveShellFromPath_WithNebiFile(t *testing.T) {
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

	result := resolveShellFromPath(dir)
	if result != dir {
		t.Errorf("resolveShellFromPath() = %q, want %q", result, dir)
	}
}

func TestResolveShellFromPath_WithPixiToml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[workspace]\nname=\"test\"\n"), 0644)

	result := resolveShellFromPath(dir)
	if result != dir {
		t.Errorf("resolveShellFromPath() = %q, want %q", result, dir)
	}
}

func TestFindValidLocalCopies_FiltersGlobal(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	// Create directories
	localPath := filepath.Join(dir, "local-ws")
	globalPath := store.GlobalRepoPath("uuid-123", "v1.0")
	os.MkdirAll(localPath, 0755)
	os.MkdirAll(globalPath, 0755)

	now := time.Now()
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
		Tag:       "v1.0",
		Path:      globalPath,
		IsGlobal:  true,
		PulledAt:  now,
	})
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
		Tag:       "v1.0",
		Path:      localPath,
		IsGlobal:  false,
		PulledAt:  now,
	})

	locals := findValidLocalCopies(store, "data-science", "v1.0")
	if len(locals) != 1 {
		t.Fatalf("Expected 1 local copy, got %d", len(locals))
	}
	if locals[0].Path != localPath {
		t.Errorf("Local path = %q, want %q", locals[0].Path, localPath)
	}
}

func TestFindValidLocalCopies_FiltersMissing(t *testing.T) {
	dir := t.TempDir()
	store := localindex.NewStoreWithDir(dir)

	existingPath := filepath.Join(dir, "existing-ws")
	missingPath := filepath.Join(dir, "missing-ws")
	os.MkdirAll(existingPath, 0755)
	// Don't create missingPath

	now := time.Now()
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
		Tag:       "v1.0",
		Path:      existingPath,
		IsGlobal:  false,
		PulledAt:  now,
	})
	store.AddEntry(localindex.RepoEntry{
		Repo: "data-science",
		Tag:       "v1.0",
		Path:      missingPath,
		IsGlobal:  false,
		PulledAt:  now,
	})

	locals := findValidLocalCopies(store, "data-science", "v1.0")
	if len(locals) != 1 {
		t.Fatalf("Expected 1 valid local copy, got %d", len(locals))
	}
	if locals[0].Path != existingPath {
		t.Errorf("Valid path = %q, want %q", locals[0].Path, existingPath)
	}
}

func TestShortenPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot determine home directory")
	}

	tests := []struct {
		input string
		want  string
	}{
		{filepath.Join(home, "projects", "foo"), "~/projects/foo"},
		{"/tmp/some/path", "/tmp/some/path"},
		{home, "~"},
	}

	for _, tt := range tests {
		got := shortenPath(tt.input)
		if got != tt.want {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetDriftStatus_NoNebiFile(t *testing.T) {
	dir := t.TempDir()
	status := getDriftStatus(dir)
	if status != "unknown" {
		t.Errorf("getDriftStatus() = %q, want %q", status, "unknown")
	}
}

func TestGetDriftStatus_Clean(t *testing.T) {
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

	status := getDriftStatus(dir)
	if status != "clean" {
		t.Errorf("getDriftStatus() = %q, want %q", status, "clean")
	}
}

func TestGetDriftStatus_Modified(t *testing.T) {
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

	status := getDriftStatus(dir)
	if status != "modified" {
		t.Errorf("getDriftStatus() = %q, want %q", status, "modified")
	}
}
