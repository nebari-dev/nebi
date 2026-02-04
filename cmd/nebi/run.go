package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [workspace-name] [pixi-args...]",
	Short: "Run a command or task via pixi",
	Long: `Run a command or task in a pixi workspace.

With no workspace name, runs in the current directory (auto-initializes if needed).
If the first argument matches a global workspace name, runs in that workspace.
A path (with a slash) uses that local directory.
All arguments are passed through to pixi run.

The --manifest-path flag is not supported; use pixi run directly.

Examples:
  nebi run my-task                    # run a pixi task in the current directory
  nebi run data-science my-task       # run a task in a global workspace
  nebi run ./my-project my-task       # run a task in a local directory
  nebi run -e dev my-task             # run with a specific pixi environment`,
	DisableFlagParsing: true,
	RunE:               runRun,
}

func runRun(cmd *cobra.Command, args []string) error {
	if err := rejectManifestPath(args, "run"); err != nil {
		return err
	}

	dir, pixiArgs, isGlobal, err := resolveWorkspaceArgs(args)
	if err != nil {
		return err
	}

	if !isGlobal {
		if err := ensureInit(dir); err != nil {
			return err
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "pixi.toml")); err != nil {
		return fmt.Errorf("no pixi.toml found in %s", dir)
	}

	pixiPath, err := exec.LookPath("pixi")
	if err != nil {
		return fmt.Errorf("pixi not found in PATH; install it from https://pixi.sh")
	}

	fullArgs := append([]string{"run"}, pixiArgs...)
	c := exec.Command(pixiPath, fullArgs...)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("pixi run exited with code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to start pixi run: %w", err)
	}
	return nil
}
