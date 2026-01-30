package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
)

var (
	publishServer   string
	publishRegistry string
)

var workspacePublishCmd = &cobra.Command{
	Use:   "publish <workspace>:<tag> [<repo>:<oci-tag>]",
	Short: "Publish a workspace version to an OCI registry",
	Long: `Publish a workspace version from the server to an OCI registry.

The workspace must already exist on the server with the specified tag.
If --registry is not specified, the server's default registry is used.

The optional second argument specifies the OCI repository and tag.
If omitted, the workspace name and tag are used as defaults.

Examples:
  nebi workspace publish myworkspace:v1.0 -s work
  nebi workspace publish myworkspace:v1.0 -s work myorg/myenv:latest
  nebi workspace publish myworkspace:v1.0 -s work --registry ghcr myorg/myenv:latest`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runWorkspacePublish,
}

func init() {
	workspacePublishCmd.Flags().StringVarP(&publishServer, "server", "s", "", "Server name or URL (uses default if not set)")
	workspacePublishCmd.Flags().StringVar(&publishRegistry, "registry", "", "Registry name or ID (uses server default if not set)")
}

func runWorkspacePublish(cmd *cobra.Command, args []string) error {
	envName, tag := parseEnvRef(args[0])
	if tag == "" {
		return fmt.Errorf("tag is required; usage: nebi workspace publish <workspace>:<tag>")
	}

	// Parse optional repo:oci-tag from second positional arg
	repo := envName
	ociTag := tag
	if len(args) == 2 {
		r, t := parseEnvRef(args[1])
		repo = r
		if t != "" {
			ociTag = t
		}
	}

	server, err := resolveServerFlag(publishServer)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient(server)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Find environment on server
	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		return err
	}

	// Resolve registry ID
	registryID, err := resolveRegistryID(client, ctx, publishRegistry)
	if err != nil {
		return err
	}

	req := cliclient.PublishRequest{
		RegistryID: registryID,
		Repository: repo,
		Tag:        ociTag,
	}

	fmt.Fprintf(os.Stderr, "Publishing %s:%s to %s:%s...\n", envName, tag, repo, ociTag)
	resp, err := client.PublishEnvironment(ctx, env.ID, req)
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

	if registry == "" {
		for _, r := range registries {
			if r.IsDefault {
				return r.ID, nil
			}
		}
		return "", fmt.Errorf("no default registry configured on server; use --registry to specify one")
	}

	for _, r := range registries {
		if r.Name == registry || r.ID == registry {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("registry %q not found on server", registry)
}
