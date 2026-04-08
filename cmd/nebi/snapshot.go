package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var (
	snapshotMessage string
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Create a version snapshot of the current workspace",
	Long: `Create a new workspace version snapshot from pixi.toml and pixi.lock
in the current directory.

Snapshots record the workspace manifest and lock file at a point in time
so you can review history and (via the GUI) roll back to earlier states.

If the content is unchanged since the most recent snapshot, the existing
version is returned and no new record is created.

Examples:
  nebi snapshot
  nebi snapshot -m "Pinned numpy to 2.1"`,
	Args: cobra.NoArgs,
	RunE: runSnapshot,
}

func init() {
	snapshotCmd.Flags().StringVarP(&snapshotMessage, "message", "m", "", "Description of the snapshot")
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	manifestPath := filepath.Join(cwd, "pixi.toml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	// pixi.lock is optional
	lock, _ := os.ReadFile(filepath.Join(cwd, "pixi.lock"))

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	ws, err := s.FindWorkspaceByPath(cwd)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("no tracked workspace in %s; run 'nebi init' first", cwd)
	}

	description := snapshotMessage
	if description == "" {
		description = "Manual snapshot"
	}

	v, created, err := s.CreateVersion(ws.ID, string(manifest), string(lock), description)
	if err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}

	if !created {
		fmt.Fprintf(os.Stderr, "Content unchanged — reusing version %d (%s)\n",
			v.VersionNumber, v.ContentHash)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Created version %d (%s)\n", v.VersionNumber, v.ContentHash)
	return nil
}
