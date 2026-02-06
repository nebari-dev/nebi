package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/models"
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

	name := filepath.Base(cwd)
	ws := &models.Workspace{
		Name: name,
		Path: cwd,
	}
	if err := s.CreateWorkspace(ws); err != nil {
		return fmt.Errorf("saving workspace: %w", err)
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

	if _, err := os.Stat(filepath.Join(absDir, "pixi.toml")); err != nil {
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

	name := filepath.Base(absDir)
	ws := &models.Workspace{
		Name: name,
		Path: absDir,
	}
	if err := s.CreateWorkspace(ws); err != nil {
		return fmt.Errorf("saving workspace: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Tracking workspace '%s' at %s\n", name, absDir)
	return nil
}
