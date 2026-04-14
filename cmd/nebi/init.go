package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/pkgmgr/pixi"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Register current directory as a tracked workspace",
	Long:  `Registers the current directory as a nebi-tracked pixi workspace.`,
	Args:  cobra.NoArgs,
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Run pixi init if no pixi.toml exists
	if _, err := os.Stat(filepath.Join(cwd, "pixi.toml")); err != nil {
		pixiPath, err := exec.LookPath("pixi")
		if err != nil {
			return fmt.Errorf("pixi not found on PATH; install pixi first")
		}
		fmt.Fprintf(os.Stderr, "No pixi.toml found; running pixi init...\n")
		c := exec.Command(pixiPath, "init")
		c.Dir = cwd
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("pixi init failed: %w", err)
		}
	}

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	// Check if already tracked
	existing, err := s.FindWorkspaceByPath(cwd)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("workspace already tracked: %s", cwd)
	}

	// Read workspace name from pixi.toml
	pixiTomlPath := filepath.Join(cwd, "pixi.toml")
	content, err := os.ReadFile(pixiTomlPath)
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	name, err := pixi.ExtractWorkspaceName(string(content))
	if err != nil {
		return err
	}

	ws := &store.LocalWorkspace{
		Name: name,
		Path: cwd,
	}
	if err := s.CreateWorkspace(ws); err != nil {
		return fmt.Errorf("saving workspace: %w", err)
	}

	// Create an initial version snapshot so the workspace has version history
	// from the moment it is tracked. Failure here is non-fatal — the workspace
	// itself is already registered. Reuses the pixi.toml bytes already read
	// above; pixi.lock is optional and read lazily inside the helper.
	if _, err := createInitialVersion(s, ws, cwd, content, "Initial workspace tracking"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create initial version: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "Workspace '%s' initialized (%s)\n", name, cwd)
	return nil
}

// ensureInit registers dir as a tracked workspace if not already tracked.
func ensureInit(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	pixiTomlPath := filepath.Join(absDir, "pixi.toml")
	if _, err := os.Stat(pixiTomlPath); err != nil {
		return fmt.Errorf("no pixi.toml found in %s", absDir)
	}

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	existing, err := s.FindWorkspaceByPath(absDir)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	// Read workspace name from pixi.toml
	content, err := os.ReadFile(pixiTomlPath)
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	name, err := pixi.ExtractWorkspaceName(string(content))
	if err != nil {
		return err
	}

	ws := &store.LocalWorkspace{
		Name: name,
		Path: absDir,
	}
	if err := s.CreateWorkspace(ws); err != nil {
		return fmt.Errorf("saving workspace: %w", err)
	}

	if _, err := createInitialVersion(s, ws, absDir, content, "Initial workspace tracking"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create initial version: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "Tracking workspace '%s' at %s\n", name, absDir)
	return nil
}

// createInitialVersion creates a workspace version snapshot from the supplied
// pixi.toml bytes and the pixi.lock at wsPath. pixi.lock is optional — if
// absent (e.g. for a freshly created workspace before `pixi install`) an
// empty lock is recorded.
func createInitialVersion(s *store.Store, ws *store.LocalWorkspace, wsPath string, manifest []byte, description string) (*store.LocalWorkspaceVersion, error) {
	lock, _ := os.ReadFile(filepath.Join(wsPath, "pixi.lock"))
	v, _, err := s.CreateVersion(ws.ID, string(manifest), string(lock), description)
	if err != nil {
		return nil, err
	}
	return v, nil
}
