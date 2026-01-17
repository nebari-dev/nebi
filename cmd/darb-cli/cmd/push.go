package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
)

var pushTags []string
var pushRegistry string

var pushCmd = &cobra.Command{
	Use:   "push <env>",
	Short: "Push environment to registry",
	Long: `Push an environment to an OCI registry with one or more tags.

Examples:
  # Push with single tag
  darb push myenv -t v1.0.0 -r ds-team

  # Push with multiple tags
  darb push myenv -t v1.0.0 -t latest -t stable -r ds-team

  # Push using default registry
  darb push myenv -t v1.0.0`,
	Args: cobra.ExactArgs(1),
	Run:  runPush,
}

func init() {
	rootCmd.AddCommand(pushCmd)
	pushCmd.Flags().StringArrayVarP(&pushTags, "tag", "t", nil, "Tag(s) for the environment (repeatable)")
	pushCmd.Flags().StringVarP(&pushRegistry, "registry", "r", "", "Named registry (optional if default set)")
	pushCmd.MarkFlagRequired("tag")
}

func runPush(cmd *cobra.Command, args []string) {
	envName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(apiClient, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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

	// Use env name as repository
	repository := envName

	// Push each tag
	for _, tag := range pushTags {
		req := client.HandlersPublishRequest{
			RegistryId: registry.GetId(),
			Repository: repository,
			Tag:        tag,
		}

		resp, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdPublishPost(ctx, env.GetId()).Request(req).Execute()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to push tag %q: %v\n", tag, err)
			os.Exit(1)
		}

		fmt.Printf("Pushed %s:%s\n", repository, tag)
		if digest := resp.GetDigest(); digest != "" {
			fmt.Printf("  Digest: %s\n", digest)
		}
	}

	fmt.Printf("\nSuccessfully pushed %d tag(s) to %s\n", len(pushTags), registry.GetName())
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
