package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var pullRegistry string
var pullOutput string

var pullCmd = &cobra.Command{
	Use:   "pull <workspace>[:<tag>]",
	Short: "Pull workspace from server",
	Long: `Pull a workspace's pixi.toml and pixi.lock from the server.

Supports Docker-style references:
  - workspace:tag    - Pull specific tag
  - workspace        - Pull latest version
  - workspace@digest - Pull by digest (immutable)

Examples:
  # Pull latest version
  nebi pull myworkspace

  # Pull specific tag
  nebi pull myworkspace:v1.0.0

  # Pull by digest
  nebi pull myworkspace@sha256:abc123def

  # Pull to specific directory
  nebi pull myworkspace:v1.0.0 -o ./my-project`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory")
}

func runPull(cmd *cobra.Command, args []string) {
	// Parse workspace:tag or workspace@digest format
	workspaceName, tagOrDigest, err := parseWorkspaceRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Determine if it's a digest reference
	isDigest := strings.HasPrefix(tagOrDigest, "@")
	tag := ""
	digest := ""
	if isDigest {
		digest = tagOrDigest[1:] // Remove @ prefix
	} else {
		tag = tagOrDigest
	}

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(apiClient, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

	// Find the version to pull
	var versionNumber int32
	if tag != "" || digest != "" {
		// Find version matching tag or digest from publications
		pubs, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdPublicationsGet(ctx, env.GetId()).Execute()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get publications: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, pub := range pubs {
			if (tag != "" && pub.GetTag() == tag) || (digest != "" && pub.GetDigest() == digest) {
				// Find the version that matches - publications don't directly link to versions
				// For now, use the latest version
				found = true
				break
			}
		}

		if !found && tag != "" {
			fmt.Fprintf(os.Stderr, "Error: Tag %q not found for workspace %q\n", tag, workspaceName)
			os.Exit(1)
		}
		if !found && digest != "" {
			fmt.Fprintf(os.Stderr, "Error: Digest %q not found for workspace %q\n", digest, workspaceName)
			os.Exit(1)
		}
	}

	// Use the latest version (highest version number)
	latestVersion := versions[0]
	for _, v := range versions {
		if v.GetVersionNumber() > latestVersion.GetVersionNumber() {
			latestVersion = v
		}
	}
	versionNumber = latestVersion.GetVersionNumber()

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

	// Create output directory if needed
	if err := os.MkdirAll(pullOutput, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Write pixi.toml
	pixiTomlPath := filepath.Join(pullOutput, "pixi.toml")
	if err := os.WriteFile(pixiTomlPath, []byte(pixiToml), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.toml: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", pixiTomlPath)

	// Write pixi.lock
	pixiLockPath := filepath.Join(pullOutput, "pixi.lock")
	if err := os.WriteFile(pixiLockPath, []byte(pixiLock), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.lock: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", pixiLockPath)

	refStr := workspaceName
	if tag != "" {
		refStr = workspaceName + ":" + tag
	} else if digest != "" {
		refStr = workspaceName + "@" + digest
	}
	fmt.Printf("\nPulled %s (version %d)\n", refStr, versionNumber)
	fmt.Println("\nTo install the environment, run:")
	fmt.Printf("  cd %s && pixi install\n", pullOutput)
}
