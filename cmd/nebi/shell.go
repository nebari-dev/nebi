package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var shellPixiEnv string
var shellPath string

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Activate environment shell via pixi",
	Long: `Activate an interactive shell in a pixi workspace.

By default, uses the current directory. Use -C to specify a different path.

Examples:
  # Shell in current directory
  nebi shell

  # Shell into a specific pixi environment
  nebi shell -e dev

  # Shell at a specific path
  nebi shell -C ~/my-project`,
	Args: cobra.NoArgs,
	RunE: runShell,
}

func init() {
	shellCmd.Flags().StringVarP(&shellPixiEnv, "env", "e", "", "Pixi environment name")
	shellCmd.Flags().StringVarP(&shellPath, "path", "C", "", "Directory containing pixi.toml")
}

func runShell(cmd *cobra.Command, args []string) error {
	dir := "."
	if shellPath != "" {
		dir = shellPath
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Verify pixi.toml exists
	if _, err := os.Stat(filepath.Join(absDir, "pixi.toml")); err != nil {
		return fmt.Errorf("no pixi.toml found in %s", absDir)
	}

	// Find pixi binary
	pixiPath, err := exec.LookPath("pixi")
	if err != nil {
		return fmt.Errorf("pixi not found in PATH; install it from https://pixi.sh")
	}

	// Build pixi shell command
	pixiArgs := []string{"shell"}
	if shellPixiEnv != "" {
		pixiArgs = append(pixiArgs, "-e", shellPixiEnv)
	}

	c := exec.Command(pixiPath, pixiArgs...)
	c.Dir = absDir
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
