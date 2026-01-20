package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openteams-ai/darb/cli/client"
	"github.com/spf13/cobra"
)

var pushRegistry string

var pushCmd = &cobra.Command{
	Use:   "push <repo>:<tag>",
	Short: "Push repo to registry",
	Long: `Push a repo to an OCI registry with a tag.

Looks for pixi.toml and pixi.lock in the current directory.
If the repo doesn't exist on the server, it will be created automatically.

Examples:
  # Push with tag
  darb push myrepo:v1.0.0 -r ds-team

  # Push using default registry
  darb push myrepo:v1.0.0`,
	Args: cobra.ExactArgs(1),
	Run:  runPush,
}

func init() {
	rootCmd.AddCommand(pushCmd)
	pushCmd.Flags().StringVarP(&pushRegistry, "registry", "r", "", "Named registry (optional if default set)")
}

func runPush(cmd *cobra.Command, args []string) {
	// Parse repo:tag format
	repoName, tag, err := parseRepoRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: darb push <repo>:<tag>")
		os.Exit(1)
	}

	if tag == "" {
		fmt.Fprintf(os.Stderr, "Error: tag is required\n")
		fmt.Fprintln(os.Stderr, "Usage: darb push <repo>:<tag>")
		os.Exit(1)
	}

	// Check for local pixi.toml
	pixiTomlContent, err := os.ReadFile("pixi.toml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: pixi.toml not found in current directory\n")
		fmt.Fprintln(os.Stderr, "Run 'pixi init' to create a pixi project first")
		os.Exit(1)
	}

	// Check for local pixi.lock (optional but recommended)
	if _, err := os.Stat("pixi.lock"); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Warning: pixi.lock not found. Run 'pixi install' to generate it.")
	}

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Try to find repo by name, create if not found
	env, err := findRepoByName(apiClient, ctx, repoName)
	if err != nil {
		// Repo doesn't exist, create it
		fmt.Printf("Creating repo %q...\n", repoName)
		pixiTomlStr := string(pixiTomlContent)
		pkgMgr := "pixi"
		createReq := client.HandlersCreateEnvironmentRequest{
			Name:           repoName,
			PackageManager: &pkgMgr,
			PixiToml:       &pixiTomlStr,
		}

		newEnv, _, createErr := apiClient.EnvironmentsAPI.EnvironmentsPost(ctx).Environment(createReq).Execute()
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to create repo %q: %v\n", repoName, createErr)
			os.Exit(1)
		}
		fmt.Printf("Created repo %q\n", repoName)

		// Wait for environment to be ready
		env, err = waitForEnvReady(apiClient, ctx, newEnv.GetId(), 60*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Find registry
	var registry *client.HandlersRegistryResponse
	if pushRegistry != "" {
		registry, err = findRegistryByName(apiClient, ctx, pushRegistry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Find default registry
		registry, err = findDefaultRegistry(apiClient, ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintln(os.Stderr, "Hint: Set a default registry with 'darb registry set-default <name>' or specify one with -r")
			os.Exit(1)
		}
	}

	// Use repo name as repository
	repository := repoName

	req := client.HandlersPublishRequest{
		RegistryId: registry.GetId(),
		Repository: repository,
		Tag:        tag,
	}

	fmt.Printf("Pushing %s:%s to %s...\n", repository, tag, registry.GetName())
	resp, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdPublishPost(ctx, env.GetId()).Request(req).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to push %s:%s: %v\n", repository, tag, err)
		os.Exit(1)
	}

	fmt.Printf("Pushed %s:%s\n", repository, tag)
	if digest := resp.GetDigest(); digest != "" {
		fmt.Printf("  Digest: %s\n", digest)
	}
	fmt.Printf("\nSuccessfully pushed to %s\n", registry.GetName())
}

// parseRepoRef parses a reference in the format repo:tag or repo@digest
// Returns (repo, tag, error) for tag references
// Returns (repo, "", error) for digest references (digest is in tag field with @ prefix)
func parseRepoRef(ref string) (repo string, tag string, err error) {
	// Check for digest reference first (repo@sha256:...)
	if idx := strings.Index(ref, "@"); idx != -1 {
		return ref[:idx], ref[idx:], nil // Return @sha256:... as the "tag"
	}

	// Check for tag reference (repo:tag)
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:], nil
	}

	// No tag or digest specified
	return ref, "", nil
}

// findDefaultRegistry finds the default registry
func findDefaultRegistry(apiClient *client.APIClient, ctx context.Context) (*client.HandlersRegistryResponse, error) {
	registries, _, err := apiClient.AdminAPI.AdminRegistriesGet(ctx).Execute()
	if err != nil {
		// Try public endpoint
		registries, _, err = apiClient.RegistriesAPI.RegistriesGet(ctx).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to list registries: %v", err)
		}
	}

	for _, reg := range registries {
		if reg.GetIsDefault() {
			return &reg, nil
		}
	}

	return nil, fmt.Errorf("no default registry set")
}

// waitForEnvReady polls until the environment is ready or timeout
func waitForEnvReady(apiClient *client.APIClient, ctx context.Context, envID string, timeout time.Duration) (*client.ModelsEnvironment, error) {
	deadline := time.Now().Add(timeout)
	fmt.Print("Waiting for environment to be ready")

	for time.Now().Before(deadline) {
		env, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdGet(ctx, envID).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to get environment status: %v", err)
		}

		status := env.GetStatus()
		switch status {
		case "ready":
			fmt.Println(" done")
			return env, nil
		case "failed", "error":
			fmt.Println(" failed")
			return nil, fmt.Errorf("environment setup failed")
		default:
			fmt.Print(".")
			time.Sleep(500 * time.Millisecond)
		}
	}

	fmt.Println(" timeout")
	return nil, fmt.Errorf("timeout waiting for environment to be ready")
}
