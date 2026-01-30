package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show changes since last commit",
	Long:  `Compares the current pixi.toml and pixi.lock against the last committed snapshots.`,
	Args:  cobra.NoArgs,
	RunE:  runDiff,
}

func runDiff(cmd *cobra.Command, args []string) error {
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
	if status == localstore.StatusMissing {
		return fmt.Errorf("workspace directory or pixi.toml is missing")
	}
	if status == localstore.StatusClean {
		fmt.Fprintln(os.Stderr, "No changes.")
		return nil
	}

	// Diff each spec file
	hasOutput := false
	for _, name := range localstore.SpecFiles {
		current := filepath.Join(cwd, name)
		snapshot := filepath.Join(store.SnapshotDir(ws.ID), name)

		// Check which files exist
		_, errC := os.Stat(current)
		_, errS := os.Stat(snapshot)

		if os.IsNotExist(errC) && os.IsNotExist(errS) {
			continue
		}

		if os.IsNotExist(errS) {
			fmt.Printf("--- new file: %s\n", name)
			hasOutput = true
			continue
		}

		if os.IsNotExist(errC) {
			fmt.Printf("--- deleted file: %s\n", name)
			hasOutput = true
			continue
		}

		// Both exist — run diff
		out, err := exec.Command("diff", "-u",
			"--label", "committed/"+name,
			"--label", "current/"+name,
			snapshot, current,
		).Output()

		if err != nil {
			// diff exits 1 when files differ, which is expected
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				fmt.Print(string(out))
				hasOutput = true
				continue
			}
			return fmt.Errorf("running diff on %s: %w", name, err)
		}
		// exit 0 means no differences (shouldn't reach here given status check)
	}

	if !hasOutput {
		fmt.Fprintln(os.Stderr, "No changes.")
	}

	return nil
}
