package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

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

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var versionNumber int32

	if tag != "" || digest != "" {
		// Find the publication matching the tag or digest
		pubs, err := client.GetEnvironmentPublications(ctx, env.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get publications: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, pub := range pubs {
			if (tag != "" && pub.Tag == tag) || (digest != "" && pub.Digest == digest) {
				versionNumber = int32(pub.VersionNumber)
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
	} else {
		// No tag/digest specified, get the latest version
		versions, err := client.GetEnvironmentVersions(ctx, env.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
			os.Exit(1)
		}

		if len(versions) == 0 {
			fmt.Fprintf(os.Stderr, "Error: Workspace %q has no versions\n", workspaceName)
			os.Exit(1)
		}

		// Use the latest version (highest version number)
		latestVersion := versions[0]
		for _, v := range versions {
			if v.VersionNumber > latestVersion.VersionNumber {
				latestVersion = v
			}
		}
		versionNumber = latestVersion.VersionNumber
	}

	// Get pixi.toml
	pixiToml, err := client.GetVersionPixiToml(ctx, env.ID, versionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.toml: %v\n", err)
		os.Exit(1)
	}

	// Get pixi.lock
	pixiLock, err := client.GetVersionPixiLock(ctx, env.ID, versionNumber)
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
