package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Snapshot current spec files",
	Long:  `Overwrites the stored spec file snapshots with the current filesystem versions of pixi.toml and pixi.lock.`,
	Args:  cobra.NoArgs,
	RunE:  runCommit,
}

func runCommit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	ws, ok := idx.Workspaces[cwd]
	if !ok {
		return fmt.Errorf("current directory is not a tracked workspace; run 'nebi init' first")
	}

	status := store.ComputeStatus(ws)
	if status == localstore.StatusClean {
		fmt.Fprintln(os.Stderr, "Nothing to commit; workspace is clean.")
		return nil
	}
	if status == localstore.StatusMissing {
		return fmt.Errorf("workspace directory or pixi.toml is missing")
	}

	if err := store.SaveSnapshot(ws.ID, cwd); err != nil {
		return fmt.Errorf("saving snapshot: %w", err)
	}

	ws.UpdatedAt = time.Now()
	if err := store.SaveIndex(idx); err != nil {
		return fmt.Errorf("saving index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Committed snapshot for '%s'\n", ws.Name)
	return nil
}
