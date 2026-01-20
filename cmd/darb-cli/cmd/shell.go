package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var shellPixiEnv string

var shellCmd = &cobra.Command{
	Use:   "shell <workspace>[:<tag>]",
	Short: "Activate workspace shell",
	Long: `Activate a workspace shell using pixi shell.

The workspace is pulled from the server and cached locally.

Examples:
  # Shell into latest version
  nebi shell myworkspace

  # Shell into specific tag
  nebi shell myworkspace:v1.0.0

  # Shell into specific pixi environment
  nebi shell myworkspace:v1.0.0 -e dev`,
	Args: cobra.ExactArgs(1),
	Run:  runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellPixiEnv, "env", "e", "", "Pixi environment name")
}

func runShell(cmd *cobra.Command, args []string) {
	// Parse workspace:tag format
	workspaceName, tag, err := parseWorkspaceRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(apiClient, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create cache directory for this workspace (include tag in path if specified)
	cacheName := workspaceName
	if tag != "" {
		cacheName = workspaceName + "-" + tag
	}
	cacheDir, err := getWorkspaceCacheDir(cacheName)
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
		fmt.Fprintf(os.Stderr, "Error: Workspace %q has no versions\n", workspaceName)
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

	refStr := workspaceName
	if tag != "" {
		refStr = workspaceName + ":" + tag
	}

	if needsUpdate {
		fmt.Printf("Pulling %s (version %d)...\n", refStr, versionNumber)

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
	fmt.Printf("Starting shell for %s...\n", refStr)

	pixiArgs := []string{"shell"}
	if shellPixiEnv != "" {
		pixiArgs = append(pixiArgs, "-e", shellPixiEnv)
	}

	pixiCmd := exec.Command("pixi", pixiArgs...)
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

// getWorkspaceCacheDir returns the cache directory for a workspace
func getWorkspaceCacheDir(workspaceName string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	workspaceDir := filepath.Join(cacheDir, "nebi", "workspaces", workspaceName)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", err
	}

	return workspaceDir, nil
}
