package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/localstore"
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

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	// Check if already tracked
	if _, exists := idx.Workspaces[cwd]; exists {
		return fmt.Errorf("workspace already tracked: %s", cwd)
	}

	// Create workspace entry
	id := uuid.New().String()
	name := filepath.Base(cwd)

	idx.Workspaces[cwd] = &localstore.Workspace{
		ID:   id,
		Name: name,
		Path: cwd,
	}

	if err := store.SaveIndex(idx); err != nil {
		return fmt.Errorf("saving index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Workspace '%s' initialized (%s)\n", name, cwd)
	return nil
}

// ensureInit registers dir as a tracked workspace if not already tracked.
// No-op if already tracked. Prints a message to stderr on new registration.
func ensureInit(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if _, err := os.Stat(filepath.Join(absDir, "pixi.toml")); err != nil {
		return fmt.Errorf("no pixi.toml found in %s", absDir)
	}

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	if _, exists := idx.Workspaces[absDir]; exists {
		return nil
	}

	id := uuid.New().String()
	name := filepath.Base(absDir)

	idx.Workspaces[absDir] = &localstore.Workspace{
		ID:   id,
		Name: name,
		Path: absDir,
	}

	if err := store.SaveIndex(idx); err != nil {
		return fmt.Errorf("saving index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Tracking workspace '%s' at %s\n", name, absDir)
	return nil
}
