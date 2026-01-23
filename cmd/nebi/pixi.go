package main

import (
	"fmt"
	"os"
	"os/exec"
)

// pixiBinary resolves the pixi binary path, returning an error if not found.
func pixiBinary() (string, error) {
	path, err := exec.LookPath("pixi")
	if err != nil {
		return "", fmt.Errorf("pixi not found in PATH. Install it from https://pixi.sh")
	}
	return path, nil
}

// mustPixiBinary returns the pixi binary path or exits with an error.
func mustPixiBinary() string {
	path, err := pixiBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return path
}

// runPixiInstall runs "pixi install --frozen" in the given directory.
// It streams stdout/stderr to the user. Returns an error if the install fails.
func runPixiInstall(dir string) error {
	pixiPath := mustPixiBinary()

	cmd := exec.Command(pixiPath, "install", "--frozen")
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Installing environment (pixi install --frozen)...\n")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("pixi install failed (exit code %d)", exitErr.ExitCode())
		}
		return fmt.Errorf("pixi install failed: %v", err)
	}
	return nil
}

// execPixiShell runs "pixi shell" in the given directory with optional environment name.
// This replaces the current process's stdin/stdout/stderr with pixi's.
func execPixiShell(dir string, envName string) {
	pixiPath := mustPixiBinary()

	args := []string{"shell"}
	if envName != "" {
		args = append(args, "-e", envName)
	}

	cmd := exec.Command(pixiPath, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error: Failed to start pixi shell: %v\n", err)
		os.Exit(1)
	}
}
