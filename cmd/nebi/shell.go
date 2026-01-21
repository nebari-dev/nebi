package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/spf13/cobra"
)

var shellPixiEnv string
var shellPath string
var shellRegistry string

var shellCmd = &cobra.Command{
	Use:   "shell [<workspace>[:<tag>]]",
	Short: "Activate workspace shell",
	Long: `Activate a workspace shell using pixi shell.

With no arguments: looks for local pixi.toml in current directory.
With workspace:tag: uses centrally stored environment (auto-pulls if needed).
With --path: uses environment at specified path.

Examples:
  # Use local pixi.toml in current directory
  nebi shell

  # Activate a centrally stored environment
  nebi shell data-science:v1.0.0 -r ds-team

  # Activate with specific pixi environment
  nebi shell data-science:v1.0.0 -e dev

  # Use environment at specific path
  nebi shell --path ~/projects/data-science`,
	Args: cobra.MaximumNArgs(1),
	Run:  runShell,
}

func init() {
	shellCmd.Flags().StringVarP(&shellPixiEnv, "env", "e", "", "Pixi environment name")
	shellCmd.Flags().StringVar(&shellPath, "path", "", "Path to environment directory")
	shellCmd.Flags().StringVarP(&shellRegistry, "registry", "r", "", "Named registry")
}

func runShell(cmd *cobra.Command, args []string) {
	var envDir string
	var err error

	// Determine which mode we're in
	if shellPath != "" {
		// Mode 1: Explicit path provided
		envDir, err = expandPath(shellPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to resolve path: %v\n", err)
			os.Exit(1)
		}

		if !envExistsLocally(envDir) {
			fmt.Fprintf(os.Stderr, "Error: No pixi.toml found at %s\n", envDir)
			os.Exit(1)
		}

		fmt.Printf("Starting shell in %s...\n", envDir)

	} else if len(args) == 0 {
		// Mode 2: No args - look for local pixi.toml
		var found bool
		envDir, found = findLocalPixiToml()
		if !found {
			fmt.Fprintln(os.Stderr, "Error: No pixi.toml found in current directory")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Hint: Run 'pixi init' to create a local environment, or")
			fmt.Fprintln(os.Stderr, "      specify a workspace with 'nebi shell workspace:tag'")
			os.Exit(1)
		}

		fmt.Println("Starting shell for local environment...")

	} else {
		// Mode 3: workspace:tag provided - use central storage
		workspaceName, tag, err := parseWorkspaceRef(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		client := mustGetClient()
		ctx := mustGetAuthContext()

		// Determine registry
		var registry *cliclient.Registry
		if shellRegistry != "" {
			registry, err = findRegistryByName(client, ctx, shellRegistry)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Find default registry
			registry, err = findDefaultRegistry(client, ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				fmt.Fprintln(os.Stderr, "Hint: Set a default registry with 'nebi registry set-default <name>' or specify one with -r")
				os.Exit(1)
			}
		}

		effectiveTag := tag
		if effectiveTag == "" {
			effectiveTag = "latest"
		}

		// Find workspace by name to get UUID
		env, err := findWorkspaceByName(client, ctx, workspaceName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Load index and get/create workspace UUID mapping
		index, err := loadIndex()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load index: %v\n", err)
			os.Exit(1)
		}

		workspaceUUID := getOrCreateWorkspaceUUID(index, workspaceName, env.ID)

		// Get central env path
		envDir, err = getCentralEnvPath(workspaceUUID, registry.Name, effectiveTag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to determine storage path: %v\n", err)
			os.Exit(1)
		}

		// Check if env exists locally, if not, pull it
		if !envExistsLocally(envDir) {
			refStr := workspaceName
			if tag != "" {
				refStr = workspaceName + ":" + tag
			}
			fmt.Printf("Environment not found locally, pulling %s...\n", refStr)

			// Pull the environment
			if err := pullEnvironmentToPath(client, ctx, env, envDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to pull environment: %v\n", err)
				os.Exit(1)
			}

			// Save the index (we already have the mapping)
			if err := saveIndex(index); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to save index: %v\n", err)
			}
		}

		refStr := workspaceName
		if tag != "" {
			refStr = workspaceName + ":" + tag
		}
		fmt.Printf("Starting shell for %s...\n", refStr)
	}

	// Run pixi shell
	pixiArgs := []string{"shell"}
	if shellPixiEnv != "" {
		pixiArgs = append(pixiArgs, "-e", shellPixiEnv)
	}

	pixiCmd := exec.Command("pixi", pixiArgs...)
	pixiCmd.Dir = envDir
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

// pullEnvironmentToPath pulls an environment's pixi.toml and pixi.lock to a specific path.
func pullEnvironmentToPath(client *cliclient.Client, ctx context.Context, env *cliclient.Environment, outputDir string) error {
	// Get versions to find the latest
	versions, err := client.GetEnvironmentVersions(ctx, env.ID)
	if err != nil {
		return fmt.Errorf("failed to get versions: %v", err)
	}

	if len(versions) == 0 {
		return fmt.Errorf("workspace has no versions")
	}

	// Use the latest version (highest version number)
	latestVersion := versions[0]
	for _, v := range versions {
		if v.VersionNumber > latestVersion.VersionNumber {
			latestVersion = v
		}
	}
	versionNumber := latestVersion.VersionNumber

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Get and write pixi.toml
	pixiToml, err := client.GetVersionPixiToml(ctx, env.ID, versionNumber)
	if err != nil {
		return fmt.Errorf("failed to get pixi.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "pixi.toml"), []byte(pixiToml), 0644); err != nil {
		return fmt.Errorf("failed to write pixi.toml: %v", err)
	}

	// Get and write pixi.lock
	pixiLock, err := client.GetVersionPixiLock(ctx, env.ID, versionNumber)
	if err != nil {
		return fmt.Errorf("failed to get pixi.lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "pixi.lock"), []byte(pixiLock), 0644); err != nil {
		return fmt.Errorf("failed to write pixi.lock: %v", err)
	}

	fmt.Printf("Pulled to %s\n", outputDir)
	return nil
}
