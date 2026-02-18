package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var shellCmd = &cobra.Command{
	Use:   "shell [workspace-name] [pixi-args...]",
	Short: "Activate workspace shell via pixi",
	Long: `Activate an interactive shell in a pixi workspace.

With no arguments, activates the current directory (auto-initializes if needed).
A bare name that matches a tracked workspace uses that workspace.
If multiple workspaces share the same name, an interactive picker is shown.
A path (with a slash) uses that local directory.
All arguments are passed through to pixi shell.

The --manifest-path flag is managed by nebi; use pixi shell directly if you need custom manifest paths.

Named workspaces activate via --manifest-path so you stay in your current directory.

Examples:
  nebi shell                       # shell in current directory
  nebi shell data-science          # activate a workspace by name (stays in cwd)
  nebi shell ./my-project          # shell into a local directory
  nebi shell data-science -e dev   # activate with a specific pixi environment`,
	DisableFlagParsing: true,
	RunE:               runShell,
}

func runShell(cmd *cobra.Command, args []string) error {
	if err := rejectManifestPath(args, "shell"); err != nil {
		return err
	}

	dir, pixiArgs, useManifestPath, err := resolveWorkspaceArgs(args)
	if err != nil {
		return err
	}

	if !useManifestPath {
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

	fullArgs := []string{"shell"}
	if useManifestPath {
		fullArgs = append(fullArgs, "--manifest-path", filepath.Join(dir, "pixi.toml"))
	}
	fullArgs = append(fullArgs, pixiArgs...)
	c := exec.Command(pixiPath, fullArgs...)
	if !useManifestPath {
		c.Dir = dir
	}
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
// Returns: directory path, remaining pixi args, whether to use --manifest-path, error.
func resolveWorkspaceArgs(args []string) (dir string, pixiArgs []string, useManifestPath bool, err error) {
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

	// Check if first arg is a workspace name
	s, err := store.New()
	if err != nil {
		return "", nil, false, err
	}
	defer s.Close()

	workspaces, err := findWorkspacesByNameWithSync(s, first)
	if err != nil {
		return "", nil, false, err
	}

	switch len(workspaces) {
	case 0:
		// Not a workspace — all args are pixi args, use cwd
		cwd, err := os.Getwd()
		if err != nil {
			return "", nil, false, fmt.Errorf("getting working directory: %w", err)
		}
		return cwd, args, false, nil
	case 1:
		// Single match — use it
		return workspaces[0].Path, rest, true, nil
	default:
		// Multiple matches — show interactive picker
		ws, err := pickWorkspace(workspaces, first)
		if err != nil {
			return "", nil, false, err
		}
		return ws.Path, rest, true, nil
	}
}

// pickWorkspace prompts the user to select from multiple workspaces with the same name.
// In non-interactive mode (piped stdin), returns an error asking the user to use a path.
func pickWorkspace(workspaces []store.LocalWorkspace, name string) (*store.LocalWorkspace, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		paths := make([]string, len(workspaces))
		for i, ws := range workspaces {
			paths[i] = ws.Path
		}
		return nil, fmt.Errorf("multiple workspaces named %q; use a path to disambiguate:\n  %s", name, strings.Join(paths, "\n  "))
	}

	fmt.Fprintf(os.Stderr, "Multiple workspaces named %q:\n", name)
	for i, ws := range workspaces {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, ws.Path)
	}
	fmt.Fprintf(os.Stderr, "Select [1-%d]: ", len(workspaces))

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading selection: %w", err)
	}

	input = strings.TrimSpace(input)
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(workspaces) {
		return nil, fmt.Errorf("invalid selection: %s", input)
	}

	return &workspaces[choice-1], nil
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
