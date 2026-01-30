package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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

	// Verify pixi.toml exists
	if _, err := os.Stat(filepath.Join(cwd, "pixi.toml")); err != nil {
		return fmt.Errorf("pixi.toml not found in current directory; not a pixi workspace")
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
	now := time.Now()

	idx.Workspaces[cwd] = &localstore.Workspace{
		ID:        id,
		Name:      name,
		Path:      cwd,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.SaveIndex(idx); err != nil {
		return fmt.Errorf("saving index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Workspace '%s' initialized (%s)\n", name, cwd)
	return nil
}
