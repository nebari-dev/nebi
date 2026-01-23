package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPixiBinary_Found(t *testing.T) {
	// This test depends on pixi being in PATH (skip if not available)
	if _, err := exec.LookPath("pixi"); err != nil {
		t.Skip("pixi not in PATH, skipping")
	}

	path, err := pixiBinary()
	if err != nil {
		t.Fatalf("pixiBinary() error = %v", err)
	}
	if path == "" {
		t.Fatal("pixiBinary() returned empty path")
	}
}

func TestPixiBinary_NotFound(t *testing.T) {
	// Temporarily set PATH to empty to simulate pixi not found
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", t.TempDir()) // empty dir with no binaries
	defer os.Setenv("PATH", origPath)

	_, err := pixiBinary()
	if err == nil {
		t.Fatal("pixiBinary() should return error when pixi not in PATH")
	}
}

func TestRunPixiInstall_PixiNotInPath(t *testing.T) {
	// We can't easily test the full install flow without a real pixi project,
	// but we can test error behavior when pixi is missing.
	// This test is skipped when pixi IS available because mustPixiBinary would succeed.
	if _, err := exec.LookPath("pixi"); err == nil {
		t.Skip("pixi is in PATH; this test checks behavior when pixi is missing")
	}

	// If pixi isn't in PATH, runPixiInstall should fail via mustPixiBinary
	// (which calls os.Exit). We can't directly test os.Exit in unit tests,
	// so this is more of a documentation test.
	t.Log("pixi not in PATH - runPixiInstall would exit with error")
}

func TestRunPixiInstall_InvalidDirectory(t *testing.T) {
	// Skip if pixi not available
	if _, err := exec.LookPath("pixi"); err != nil {
		t.Skip("pixi not in PATH, skipping")
	}

	// Running pixi install in a directory without pixi.toml should fail
	dir := t.TempDir()
	err := runPixiInstall(dir)
	if err == nil {
		t.Fatal("runPixiInstall() should fail in directory without pixi.toml")
	}
}

func TestRunPixiInstall_ValidProject(t *testing.T) {
	// Skip if pixi not available
	if _, err := exec.LookPath("pixi"); err != nil {
		t.Skip("pixi not in PATH, skipping")
	}

	// Create a minimal pixi project with a lock file
	dir := t.TempDir()

	// Minimal pixi.toml
	pixiToml := `[workspace]
name = "test-install"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
`
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(pixiToml), 0644)

	// Create a minimal valid lock file (empty deps, matching manifest)
	// We use pixi to generate the lock first
	lockCmd := exec.Command("pixi", "install")
	lockCmd.Dir = dir
	if err := lockCmd.Run(); err != nil {
		t.Skipf("pixi install failed to set up test project: %v", err)
	}

	// Now test our runPixiInstall with --frozen (should be a no-op since already installed)
	err := runPixiInstall(dir)
	if err != nil {
		t.Fatalf("runPixiInstall() error = %v", err)
	}
}
