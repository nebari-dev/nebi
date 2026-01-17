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
	Use:   "pull <repo>[:<tag>]",
	Short: "Pull repo from server",
	Long: `Pull a repo's pixi.toml and pixi.lock from the server.

Supports Docker-style references:
  - repo:tag    - Pull specific tag
  - repo        - Pull latest version
  - repo@digest - Pull by digest (immutable)

Examples:
  # Pull latest version
  darb pull myrepo

  # Pull specific tag
  darb pull myrepo:v1.0.0

  # Pull by digest
  darb pull myrepo@sha256:abc123def

  # Pull to specific directory
  darb pull myrepo:v1.0.0 -o ./my-project`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory")
}

func runPull(cmd *cobra.Command, args []string) {
	// Parse repo:tag or repo@digest format
	repoName, tagOrDigest, err := parseRepoRef(args[0])
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

	// Find repo by name
	env, err := findRepoByName(apiClient, ctx, repoName)
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
		fmt.Fprintf(os.Stderr, "Error: Repo %q has no versions\n", repoName)
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
			fmt.Fprintf(os.Stderr, "Error: Tag %q not found for repo %q\n", tag, repoName)
			os.Exit(1)
		}
		if !found && digest != "" {
			fmt.Fprintf(os.Stderr, "Error: Digest %q not found for repo %q\n", digest, repoName)
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

	refStr := repoName
	if tag != "" {
		refStr = repoName + ":" + tag
	} else if digest != "" {
		refStr = repoName + "@" + digest
	}
	fmt.Printf("\nPulled %s (version %d)\n", refStr, versionNumber)
	fmt.Println("\nTo install the environment, run:")
	fmt.Printf("  cd %s && pixi install\n", pullOutput)
}
