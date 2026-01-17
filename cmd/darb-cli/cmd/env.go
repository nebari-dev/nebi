package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
)

var envListRegistry string
var envInfoRegistry string
var envListTagsRegistry string

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
	Long:  `List, delete, and inspect environments.`,
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	Long: `List environments from the server.

Examples:
  # List all environments
  darb env list`,
	Args: cobra.NoArgs,
	Run:  runEnvList,
}

var envListTagsCmd = &cobra.Command{
	Use:   "tags <env>",
	Short: "List tags for an environment",
	Long: `List all published tags for an environment.

Example:
  darb env list tags myenv`,
	Args: cobra.ExactArgs(1),
	Run:  runEnvListTags,
}

var envDeleteCmd = &cobra.Command{
	Use:   "delete <env>",
	Short: "Delete an environment",
	Long: `Delete an environment from the server.

Example:
  darb env delete myenv`,
	Args: cobra.ExactArgs(1),
	Run:  runEnvDelete,
}

var envInfoCmd = &cobra.Command{
	Use:   "info <env>",
	Short: "Show environment details",
	Long: `Show detailed information about an environment.

Example:
  darb env info myenv`,
	Args: cobra.ExactArgs(1),
	Run:  runEnvInfo,
}

func init() {
	rootCmd.AddCommand(envCmd)

	// env list
	envCmd.AddCommand(envListCmd)

	// env list tags (subcommand of list)
	envListCmd.AddCommand(envListTagsCmd)

	// env delete
	envCmd.AddCommand(envDeleteCmd)

	// env info
	envCmd.AddCommand(envInfoCmd)
}

func runEnvList(cmd *cobra.Command, args []string) {
	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	envs, _, err := apiClient.EnvironmentsAPI.EnvironmentsGet(ctx).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list environments: %v\n", err)
		os.Exit(1)
	}

	if len(envs) == 0 {
		fmt.Println("No environments found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tPACKAGE MANAGER\tOWNER")
	for _, env := range envs {
		owner := ""
		if env.Owner != nil {
			owner = env.Owner.GetUsername()
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			env.GetName(),
			env.GetStatus(),
			env.GetPackageManager(),
			owner,
		)
	}
	w.Flush()
}

func runEnvListTags(cmd *cobra.Command, args []string) {
	envName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(apiClient, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get publications (tags)
	pubs, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdPublicationsGet(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list tags: %v\n", err)
		os.Exit(1)
	}

	if len(pubs) == 0 {
		fmt.Printf("No published tags for %q\n", envName)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TAG\tREGISTRY\tREPOSITORY\tDIGEST\tPUBLISHED")
	for _, pub := range pubs {
		digest := pub.GetDigest()
		if len(digest) > 19 {
			digest = digest[:19] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			pub.GetTag(),
			pub.GetRegistryName(),
			pub.GetRepository(),
			digest,
			pub.GetPublishedAt(),
		)
	}
	w.Flush()
}

func runEnvDelete(cmd *cobra.Command, args []string) {
	envName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(apiClient, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Delete
	_, err = apiClient.EnvironmentsAPI.EnvironmentsIdDelete(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to delete environment: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleted environment %q\n", envName)
}

func runEnvInfo(cmd *cobra.Command, args []string) {
	envName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(apiClient, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get full details
	envDetail, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdGet(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get environment details: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Name:            %s\n", envDetail.GetName())
	fmt.Printf("ID:              %s\n", envDetail.GetId())
	fmt.Printf("Status:          %s\n", envDetail.GetStatus())
	fmt.Printf("Package Manager: %s\n", envDetail.GetPackageManager())
	if envDetail.Owner != nil {
		fmt.Printf("Owner:           %s\n", envDetail.Owner.GetUsername())
	}
	fmt.Printf("Size:            %d bytes\n", envDetail.GetSizeBytes())
	fmt.Printf("Created:         %s\n", envDetail.GetCreatedAt())
	fmt.Printf("Updated:         %s\n", envDetail.GetUpdatedAt())

	// Get packages
	packages, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdPackagesGet(ctx, env.GetId()).Execute()
	if err == nil && len(packages) > 0 {
		fmt.Printf("\nPackages (%d):\n", len(packages))
		for _, pkg := range packages {
			fmt.Printf("  - %s", pkg.GetName())
			if v := pkg.GetVersion(); v != "" {
				fmt.Printf(" (%s)", v)
			}
			fmt.Println()
		}
	}
}

// findEnvByName looks up an environment by name and returns it
func findEnvByName(apiClient *client.APIClient, ctx context.Context, name string) (*client.ModelsEnvironment, error) {
	envs, _, err := apiClient.EnvironmentsAPI.EnvironmentsGet(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %v", err)
	}

	for _, env := range envs {
		if env.GetName() == name {
			return &env, nil
		}
	}

	return nil, fmt.Errorf("environment %q not found", name)
}
