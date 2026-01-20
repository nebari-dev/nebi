package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
)

var workspaceListRegistry string
var workspaceInfoRegistry string
var workspaceListTagsRegistry string

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage workspaces",
	Long:    `List, delete, and inspect workspaces.`,
}

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List workspaces",
	Long: `List workspaces from the server.

Examples:
  # List all workspaces
  nebi workspace list`,
	Args: cobra.NoArgs,
	Run:  runWorkspaceList,
}

var workspaceListTagsCmd = &cobra.Command{
	Use:   "tags <workspace>",
	Short: "List tags for a workspace",
	Long: `List all published tags for a workspace.

Example:
  nebi workspace list tags myworkspace`,
	Args: cobra.ExactArgs(1),
	Run:  runWorkspaceListTags,
}

var workspaceDeleteCmd = &cobra.Command{
	Use:     "delete <workspace>",
	Aliases: []string{"rm"},
	Short:   "Delete a workspace",
	Long: `Delete a workspace from the server.

Example:
  nebi workspace delete myworkspace`,
	Args: cobra.ExactArgs(1),
	Run:  runWorkspaceDelete,
}

var workspaceInfoCmd = &cobra.Command{
	Use:   "info <workspace>",
	Short: "Show workspace details",
	Long: `Show detailed information about a workspace.

Example:
  nebi workspace info myworkspace`,
	Args: cobra.ExactArgs(1),
	Run:  runWorkspaceInfo,
}

func init() {
	rootCmd.AddCommand(workspaceCmd)

	// workspace list
	workspaceCmd.AddCommand(workspaceListCmd)

	// workspace list tags (subcommand of list)
	workspaceListCmd.AddCommand(workspaceListTagsCmd)

	// workspace delete
	workspaceCmd.AddCommand(workspaceDeleteCmd)

	// workspace info
	workspaceCmd.AddCommand(workspaceInfoCmd)
}

func runWorkspaceList(cmd *cobra.Command, args []string) {
	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	envs, _, err := apiClient.EnvironmentsAPI.EnvironmentsGet(ctx).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list workspaces: %v\n", err)
		os.Exit(1)
	}

	if len(envs) == 0 {
		fmt.Println("No workspaces found")
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

func runWorkspaceListTags(cmd *cobra.Command, args []string) {
	workspaceName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(apiClient, ctx, workspaceName)
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
		fmt.Printf("No published tags for %q\n", workspaceName)
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

func runWorkspaceDelete(cmd *cobra.Command, args []string) {
	workspaceName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(apiClient, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Delete
	_, err = apiClient.EnvironmentsAPI.EnvironmentsIdDelete(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to delete workspace: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleted workspace %q\n", workspaceName)
}

func runWorkspaceInfo(cmd *cobra.Command, args []string) {
	workspaceName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(apiClient, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get full details
	envDetail, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdGet(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get workspace details: %v\n", err)
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

// findWorkspaceByName looks up a workspace by name and returns it
func findWorkspaceByName(apiClient *client.APIClient, ctx context.Context, name string) (*client.ModelsEnvironment, error) {
	envs, _, err := apiClient.EnvironmentsAPI.EnvironmentsGet(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %v", err)
	}

	for _, env := range envs {
		if env.GetName() == name {
			return &env, nil
		}
	}

	return nil, fmt.Errorf("workspace %q not found", name)
}
