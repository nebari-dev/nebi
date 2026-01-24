package main

import (
	"fmt"
	"os"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/spf13/cobra"
)

var (
	publishRegistry string
	publishAs       string
)

var publishCmd = &cobra.Command{
	Use:   "publish <repo>:<tag>",
	Short: "Publish a pushed version to an OCI registry",
	Long: `Publish a previously pushed version to an OCI registry for distribution.

The version must already exist on the server (via 'nebi push').
This command distributes the version to the specified OCI registry.

Examples:
  # Publish to a named registry
  nebi publish myrepo:v1.0.0 -r ds-team

  # Publish using default registry
  nebi publish myrepo:v1.0.0

  # Publish under a different OCI repository name
  nebi publish myrepo:v1.0.0 -r ds-team --as org/custom-name`,
	Args: cobra.ExactArgs(1),
	Run:  runPublish,
}

func init() {
	publishCmd.Flags().StringVarP(&publishRegistry, "registry", "r", "", "Named registry (optional if default set)")
	publishCmd.Flags().StringVar(&publishAs, "as", "", "OCI repository name (defaults to repo name)")
}

func runPublish(cmd *cobra.Command, args []string) {
	repoName, tag, err := parseRepoRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: nebi publish <repo>:<tag>")
		osExit(1)
	}

	if tag == "" {
		fmt.Fprintf(os.Stderr, "Error: tag is required\n")
		fmt.Fprintln(os.Stderr, "Usage: nebi publish <repo>:<tag>")
		osExit(1)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find the repo on the server
	env, err := findRepoByName(client, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: repo %q not found on server\n", repoName)
		fmt.Fprintln(os.Stderr, "Hint: Run 'nebi push' first to create a version")
		osExit(1)
	}

	// Find registry
	var registry *cliclient.Registry
	if publishRegistry != "" {
		registry, err = findRegistryByName(client, ctx, publishRegistry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			osExit(1)
		}
	} else {
		registry, err = findDefaultRegistry(client, ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintln(os.Stderr, "Hint: Set a default registry or specify one with -r")
			osExit(1)
		}
	}

	// Determine OCI repository name
	repository := repoName
	if publishAs != "" {
		repository = publishAs
	}

	req := cliclient.PublishRequest{
		RegistryID: registry.ID,
		Repository: repository,
		Tag:        tag,
	}

	fmt.Printf("Publishing %s:%s to %s/%s...\n", repoName, tag, registry.Name, repository)
	resp, err := client.PublishEnvironment(ctx, env.ID, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to publish %s:%s: %v\n", repoName, tag, err)
		osExit(1)
	}

	fmt.Printf("Published %s:%s\n", repository, tag)
	if resp.Digest != "" {
		fmt.Printf("  Digest: %s\n", resp.Digest)
	}
	fmt.Printf("  Registry: %s\n", registry.Name)
}
