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

var shellPixiEnv string

var shellCmd = &cobra.Command{
	Use:   "shell [workspace-name]",
	Short: "Activate environment shell via pixi",
	Long: `Activate an interactive shell in a pixi workspace.

With no arguments, activates the current directory.
A bare name refers to a global workspace; use a path (with a slash) for a local directory.

Examples:
  nebi shell                       # shell in current directory
  nebi shell data-science          # shell into a global workspace by name
  nebi shell ./my-project          # shell into a local directory
  nebi shell data-science -e dev   # shell with a specific pixi environment`,
	Args: cobra.MaximumNArgs(1),
	RunE: runShell,
}

func init() {
	shellCmd.Flags().StringVarP(&shellPixiEnv, "env", "e", "", "Pixi environment name")
}

func runShell(cmd *cobra.Command, args []string) error {
	var dir string

	if len(args) == 0 {
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
	if shellPixiEnv != "" {
		pixiArgs = append(pixiArgs, "-e", shellPixiEnv)
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
