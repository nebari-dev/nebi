package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/spf13/cobra"
)

var pullPath string
var pullRegistry string

var pullCmd = &cobra.Command{
	Use:   "pull <workspace>[:<tag>]",
	Short: "Pull workspace from registry",
	Long: `Pull a workspace's pixi.toml and pixi.lock from the registry.

By default, environments are stored in the central location:
  ~/.local/share/nebi/envs/<workspace-uuid>/<registry>/<tag>/

Use --path to pull to a specific directory instead.

Supports Docker-style references:
  - workspace:tag    - Pull specific tag
  - workspace        - Pull latest version
  - workspace@digest - Pull by digest (immutable)

Examples:
  # Pull to central storage (default)
  nebi pull data-science:v1.0.0 -r ds-team

  # Pull latest version
  nebi pull data-science -r ds-team

  # Pull to specific directory
  nebi pull data-science:v1.0.0 --path ./my-project

  # Pull by digest
  nebi pull data-science@sha256:abc123def -r ds-team`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullPath, "path", "p", "", "Output path (default: central storage)")
	pullCmd.Flags().StringVarP(&pullRegistry, "registry", "r", "", "Named registry")
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

	// Determine registry
	var registry *cliclient.Registry
	if pullRegistry != "" {
		registry, err = findRegistryByName(client, ctx, pullRegistry)
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

	// Find workspace by name
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get versions to find the latest
	versions, err := client.GetEnvironmentVersions(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
		os.Exit(1)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Workspace %q has no versions\n", workspaceName)
		os.Exit(1)
	}

	// Find the version to pull
	if tag != "" || digest != "" {
		// Find version matching tag or digest from publications
		pubs, err := client.GetEnvironmentPublications(ctx, env.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get publications: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, pub := range pubs {
			if (tag != "" && pub.Tag == tag) || (digest != "" && pub.Digest == digest) {
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
		if v.VersionNumber > latestVersion.VersionNumber {
			latestVersion = v
		}
	}
	versionNumber := latestVersion.VersionNumber

	// Determine output directory
	var outputDir string
	useCentralStorage := pullPath == ""

	if useCentralStorage {
		// Use central storage location
		effectiveTag := tag
		if effectiveTag == "" {
			effectiveTag = "latest"
		}

		// Load and update the index
		index, err := loadIndex()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load index: %v\n", err)
			os.Exit(1)
		}

		// Map workspace name to UUID
		workspaceUUID := getOrCreateWorkspaceUUID(index, workspaceName, env.ID)

		// Save the updated index
		if err := saveIndex(index); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to save index: %v\n", err)
			os.Exit(1)
		}

		outputDir, err = getCentralEnvPath(workspaceUUID, registry.Name, effectiveTag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to determine storage path: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Use specified path
		outputDir, err = expandPath(pullPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to resolve path: %v\n", err)
			os.Exit(1)
		}
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
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Write pixi.toml
	pixiTomlPath := filepath.Join(outputDir, "pixi.toml")
	if err := os.WriteFile(pixiTomlPath, []byte(pixiToml), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.toml: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", pixiTomlPath)

	// Write pixi.lock
	pixiLockPath := filepath.Join(outputDir, "pixi.lock")
	if err := os.WriteFile(pixiLockPath, []byte(pixiLock), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.lock: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", pixiLockPath)

	// Build reference string for display
	refStr := workspaceName
	if tag != "" {
		refStr = workspaceName + ":" + tag
	} else if digest != "" {
		refStr = workspaceName + "@" + digest
	}

	fmt.Printf("\nPulled %s (version %d)\n", refStr, versionNumber)

	if useCentralStorage {
		fmt.Printf("\nEnvironment stored at: %s\n", outputDir)
		fmt.Println("\nTo activate, run:")
		fmt.Printf("  nebi shell %s\n", refStr)
	} else {
		fmt.Println("\nTo install the environment, run:")
		fmt.Printf("  cd %s && pixi install\n", outputDir)
	}
}
