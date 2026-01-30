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

var shellCmd = &cobra.Command{
	Use:   "shell [workspace-name] [pixi-args...]",
	Short: "Activate environment shell via pixi",
	Long: `Activate an interactive shell in a pixi workspace.

With no arguments, activates the current directory (auto-initializes if needed).
A bare name that matches a global workspace uses that workspace.
A path (with a slash) uses that local directory.
All arguments are passed through to pixi shell.

The --manifest-path flag is not supported; use pixi shell directly.

Examples:
  nebi shell                       # shell in current directory
  nebi shell data-science          # shell into a global workspace by name
  nebi shell ./my-project          # shell into a local directory
  nebi shell data-science -e dev   # shell with a specific pixi environment`,
	DisableFlagParsing: true,
	RunE:               runShell,
}

func runShell(cmd *cobra.Command, args []string) error {
	if err := rejectManifestPath(args, "shell"); err != nil {
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

	fullArgs := append([]string{"shell"}, pixiArgs...)
	c := exec.Command(pixiPath, fullArgs...)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("pixi shell exited with code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to start pixi shell: %w", err)
	}
	return nil
}

// resolveWorkspaceArgs parses args for shell/run commands.
// Returns: resolved directory, remaining pixi args, whether it's a global workspace, error.
func resolveWorkspaceArgs(args []string) (dir string, pixiArgs []string, isGlobal bool, err error) {
	if len(args) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return "", nil, false, fmt.Errorf("getting working directory: %w", err)
		}
		return cwd, nil, false, nil
	}

	first := args[0]
	rest := args[1:]

	// Path — contains a slash
	if strings.Contains(first, "/") || strings.Contains(first, string(filepath.Separator)) {
		absDir, err := filepath.Abs(first)
		if err != nil {
			return "", nil, false, fmt.Errorf("resolving path: %w", err)
		}
		return absDir, rest, false, nil
	}

	// Check if first arg is a global workspace name
	store, err := localstore.NewStore()
	if err != nil {
		return "", nil, false, err
	}
	idx, err := store.LoadIndex()
	if err != nil {
		return "", nil, false, err
	}
	ws := findGlobalWorkspaceByName(idx, first)
	if ws != nil {
		return ws.Path, rest, true, nil
	}

	// Not a workspace — all args are pixi args, use cwd
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, false, fmt.Errorf("getting working directory: %w", err)
	}
	return cwd, args, false, nil
}

// rejectManifestPath scans args for --manifest-path and returns an error if found.
func rejectManifestPath(args []string, cmdName string) error {
	for _, a := range args {
		if a == "--manifest-path" || strings.HasPrefix(a, "--manifest-path=") {
			return fmt.Errorf("--manifest-path cannot be used with nebi %s; use pixi %s directly", cmdName, cmdName)
		}
	}
	return nil
}
