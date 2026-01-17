package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell <env>",
	Short: "Activate environment shell",
	Long: `Activate an environment shell using pixi shell.

The environment is pulled from the server and cached locally.

Examples:
  darb shell myenv`,
	Args: cobra.ExactArgs(1),
	Run:  runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) {
	envName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(apiClient, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create cache directory for this environment
	cacheDir, err := getEnvCacheDir(envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create cache directory: %v\n", err)
		os.Exit(1)
	}

	// Get versions to find the latest
	versions, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdVersionsGet(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
		os.Exit(1)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Environment %q has no versions\n", envName)
		os.Exit(1)
	}

	// Use the latest version
	latestVersion := versions[0]
	for _, v := range versions {
		if v.GetVersionNumber() > latestVersion.GetVersionNumber() {
			latestVersion = v
		}
	}
	versionNumber := latestVersion.GetVersionNumber()

	// Check if we need to update the cached files
	pixiTomlPath := filepath.Join(cacheDir, "pixi.toml")
	pixiLockPath := filepath.Join(cacheDir, "pixi.lock")

	needsUpdate := true
	if _, err := os.Stat(pixiTomlPath); err == nil {
		// Files exist, could add version checking here
		needsUpdate = false
	}

	if needsUpdate {
		fmt.Printf("Pulling %s (version %d)...\n", envName, versionNumber)

		// Get pixi.toml
		pixiToml, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdVersionsVersionPixiTomlGet(ctx, env.GetId(), versionNumber).Execute()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.toml: %v\n", err)
			os.Exit(1)
		}

		// Get pixi.lock
		pixiLock, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdVersionsVersionPixiLockGet(ctx, env.GetId(), versionNumber).Execute()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.lock: %v\n", err)
			os.Exit(1)
		}

		// Write files
		if err := os.WriteFile(pixiTomlPath, []byte(pixiToml), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.toml: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(pixiLockPath, []byte(pixiLock), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.lock: %v\n", err)
			os.Exit(1)
		}
	}

	// Run pixi shell
	fmt.Printf("Starting shell for %s...\n", envName)

	pixiCmd := exec.Command("pixi", "shell")
	pixiCmd.Dir = cacheDir
	pixiCmd.Stdin = os.Stdin
	pixiCmd.Stdout = os.Stdout
	pixiCmd.Stderr = os.Stderr

	if err := pixiCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error: Failed to start pixi shell: %v\n", err)
		os.Exit(1)
	}
}

// getEnvCacheDir returns the cache directory for an environment
func getEnvCacheDir(envName string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	envDir := filepath.Join(cacheDir, "darb", "envs", envName)
	if err := os.MkdirAll(envDir, 0755); err != nil {
		return "", err
	}

	return envDir, nil
}
