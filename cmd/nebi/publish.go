package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
)

var (
	publishRegistry string
	publishTag      string
	publishRepo     string
)

var publishCmd = &cobra.Command{
	Use:   "publish [workspace]",
	Short: "Publish a workspace to an OCI registry",
	Long: `Publish a workspace to an OCI registry.

If no workspace name is given, the current directory's tracked workspace is used.
The repository name defaults to the workspace name.
The tag auto-increments (v1, v2, v3, ...) based on existing publications.
If --registry is not specified, the server's default registry is used.

Examples:
  nebi publish                                       # publish current directory workspace
  nebi publish myworkspace
  nebi publish myworkspace --tag v1.0.0
  nebi publish myworkspace --repo custom-name --registry ghcr`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runWorkspacePublish,
	ValidArgsFunction: completeServerWorkspaceNames,
}

func init() {
	publishCmd.Flags().StringVar(&publishRegistry, "registry", "", "Registry name or ID (uses server default if not set)")
	publishCmd.Flags().StringVar(&publishTag, "tag", "", "OCI tag (auto-increments v1, v2, ... if not set)")
	publishCmd.Flags().StringVar(&publishRepo, "repo", "", "OCI repository name (defaults to workspace name)")
}

func runWorkspacePublish(cmd *cobra.Command, args []string) error {
	var wsName string
	if len(args) == 1 {
		wsName = args[0]
	} else {
		// Resolve from current directory origin
		origin, err := lookupOrigin()
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no workspace specified and no origin set in current directory;\nusage: nebi publish [workspace]")
		}
		wsName = origin.OriginName
		fmt.Fprintf(os.Stderr, "Using workspace %q from origin\n", wsName)
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Find workspace on server
	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		return err
	}

	// Get server-computed defaults
	defaults, err := client.GetPublishDefaults(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("getting publish defaults: %w", err)
	}

	// Use flags to override defaults
	registryID := defaults.RegistryID
	if publishRegistry != "" {
		registryID, err = resolveRegistryID(client, ctx, publishRegistry)
		if err != nil {
			return err
		}
	}

	repo := defaults.Repository
	if publishRepo != "" {
		repo = publishRepo
	}

	tag := defaults.Tag
	if publishTag != "" {
		tag = publishTag
	}

	req := cliclient.PublishRequest{
		RegistryID: registryID,
		Repository: repo,
		Tag:        tag,
	}

	fmt.Fprintf(os.Stderr, "Publishing %s to %s:%s...\n", wsName, repo, tag)
	resp, err := client.PublishWorkspace(ctx, ws.ID, req)
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Published %s:%s (digest: %s)\n", resp.Repository, resp.Tag, resp.Digest)
	return nil
}

// resolveRegistryID resolves a registry name/ID or finds the default registry.
func resolveRegistryID(client *cliclient.Client, ctx context.Context, registry string) (string, error) {
	registries, err := client.ListRegistries(ctx)
	if err != nil {
		return "", fmt.Errorf("listing registries: %w", err)
	}

	for _, r := range registries {
		if r.Name == registry || r.ID == registry {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("registry %q not found on server", registry)
}
