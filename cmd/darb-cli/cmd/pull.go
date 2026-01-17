package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var pullTag string
var pullDigest string
var pullRegistry string
var pullOutput string

var pullCmd = &cobra.Command{
	Use:   "pull <env>",
	Short: "Pull environment from server",
	Long: `Pull an environment's pixi.toml and pixi.lock from the server.

Examples:
  # Pull environment to current directory
  darb pull myenv

  # Pull to specific directory
  darb pull myenv -o ./my-project`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().StringVarP(&pullTag, "tag", "t", "", "Tag to pull (default: latest version)")
	pullCmd.Flags().StringVarP(&pullDigest, "digest", "d", "", "OCI digest (immutable reference)")
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory")
}

func runPull(cmd *cobra.Command, args []string) {
	envName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(apiClient, ctx, envName)
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
		fmt.Fprintf(os.Stderr, "Error: Environment %q has no versions\n", envName)
		os.Exit(1)
	}

	// Find the version to pull
	var versionNumber int32
	if pullTag != "" || pullDigest != "" {
		// Find version matching tag or digest from publications
		pubs, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdPublicationsGet(ctx, env.GetId()).Execute()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get publications: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, pub := range pubs {
			if (pullTag != "" && pub.GetTag() == pullTag) || (pullDigest != "" && pub.GetDigest() == pullDigest) {
				// Find the version that matches - publications don't directly link to versions
				// For now, use the latest version
				found = true
				break
			}
		}

		if !found && pullTag != "" {
			fmt.Fprintf(os.Stderr, "Error: Tag %q not found for environment %q\n", pullTag, envName)
			os.Exit(1)
		}
		if !found && pullDigest != "" {
			fmt.Fprintf(os.Stderr, "Error: Digest %q not found for environment %q\n", pullDigest, envName)
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

	fmt.Printf("\nPulled %s (version %d)\n", envName, versionNumber)
	fmt.Println("\nTo install the environment, run:")
	fmt.Printf("  cd %s && pixi install\n", pullOutput)
}
