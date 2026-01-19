package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
)

var repoListRegistry string
var repoInfoRegistry string
var repoListTagsRegistry string

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage repos",
	Long:  `List, delete, and inspect repos.`,
}

var repoListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List repos",
	Long: `List repos from the server.

Examples:
  # List all repos
  darb repo list`,
	Args: cobra.NoArgs,
	Run:  runRepoList,
}

var repoListTagsCmd = &cobra.Command{
	Use:   "tags <repo>",
	Short: "List tags for a repo",
	Long: `List all published tags for a repo.

Example:
  darb repo list tags myrepo`,
	Args: cobra.ExactArgs(1),
	Run:  runRepoListTags,
}

var repoDeleteCmd = &cobra.Command{
	Use:     "delete <repo>",
	Aliases: []string{"rm"},
	Short:   "Delete a repo",
	Long: `Delete a repo from the server.

Example:
  darb repo delete myrepo`,
	Args: cobra.ExactArgs(1),
	Run:  runRepoDelete,
}

var repoInfoCmd = &cobra.Command{
	Use:   "info <repo>",
	Short: "Show repo details",
	Long: `Show detailed information about a repo.

Example:
  darb repo info myrepo`,
	Args: cobra.ExactArgs(1),
	Run:  runRepoInfo,
}

func init() {
	rootCmd.AddCommand(repoCmd)

	// repo list
	repoCmd.AddCommand(repoListCmd)

	// repo list tags (subcommand of list)
	repoListCmd.AddCommand(repoListTagsCmd)

	// repo delete
	repoCmd.AddCommand(repoDeleteCmd)

	// repo info
	repoCmd.AddCommand(repoInfoCmd)
}

func runRepoList(cmd *cobra.Command, args []string) {
	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	envs, _, err := apiClient.EnvironmentsAPI.EnvironmentsGet(ctx).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list repos: %v\n", err)
		os.Exit(1)
	}

	if len(envs) == 0 {
		fmt.Println("No repos found")
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

func runRepoListTags(cmd *cobra.Command, args []string) {
	repoName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find repo by name
	env, err := findRepoByName(apiClient, ctx, repoName)
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
		fmt.Printf("No published tags for %q\n", repoName)
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

func runRepoDelete(cmd *cobra.Command, args []string) {
	repoName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find repo by name
	env, err := findRepoByName(apiClient, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Delete
	_, err = apiClient.EnvironmentsAPI.EnvironmentsIdDelete(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to delete repo: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleted repo %q\n", repoName)
}

func runRepoInfo(cmd *cobra.Command, args []string) {
	repoName := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find repo by name
	env, err := findRepoByName(apiClient, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get full details
	envDetail, _, err := apiClient.EnvironmentsAPI.EnvironmentsIdGet(ctx, env.GetId()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get repo details: %v\n", err)
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

// findRepoByName looks up a repo by name and returns it
func findRepoByName(apiClient *client.APIClient, ctx context.Context, name string) (*client.ModelsEnvironment, error) {
	envs, _, err := apiClient.EnvironmentsAPI.EnvironmentsGet(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %v", err)
	}

	for _, env := range envs {
		if env.GetName() == name {
			return &env, nil
		}
	}

	return nil, fmt.Errorf("repo %q not found", name)
}
