package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var activateEnv string

var activateCmd = &cobra.Command{
	Use:   "activate [workspace-name]",
	Short: "Activate a workspace shell",
	Long: `Activate an interactive shell in a pixi workspace.

With no arguments, activates the current directory.
A bare name refers to a global workspace; use a path (with a slash) for a local directory.

Examples:
  nebi activate                    # activate current directory
  nebi activate data-science       # activate global workspace by name
  nebi activate ./my-project       # activate a local directory
  nebi activate data-science -e dev`,
	Args: cobra.MaximumNArgs(1),
	RunE: runActivate,
}

func init() {
	activateCmd.Flags().StringVarP(&activateEnv, "env", "e", "", "Pixi environment name")
}

func runActivate(cmd *cobra.Command, args []string) error {
	var dir string

	if len(args) == 0 {
		// No name given — use current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		dir = cwd
	} else {
		arg := args[0]

		if strings.Contains(arg, "/") || strings.Contains(arg, string(filepath.Separator)) {
			// Argument contains a slash — treat as a path
			absDir, err := filepath.Abs(arg)
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}
			dir = absDir
		} else {
			// No slash — treat as a global workspace name
			store, err := localstore.NewStore()
			if err != nil {
				return err
			}

			idx, err := store.LoadIndex()
			if err != nil {
				return err
			}

			ws := findGlobalWorkspaceByName(idx, arg)
			if ws == nil {
				return fmt.Errorf("global workspace %q not found; run 'nebi workspace list' to see available workspaces\nTo activate a local directory, use a path (e.g. ./myproject)", arg)
			}
			dir = ws.Path
		}
	}

	// Verify pixi.toml exists
	if _, err := os.Stat(filepath.Join(dir, "pixi.toml")); err != nil {
		return fmt.Errorf("no pixi.toml found in %s", dir)
	}

	pixiPath, err := exec.LookPath("pixi")
	if err != nil {
		return fmt.Errorf("pixi not found in PATH; install it from https://pixi.sh")
	}

	pixiArgs := []string{"shell"}
	if activateEnv != "" {
		pixiArgs = append(pixiArgs, "-e", activateEnv)
	}

	c := exec.Command(pixiPath, pixiArgs...)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("failed to start pixi shell: %w", err)
	}
	return nil
}
