package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage workspaces",
	Long:    `List, delete, and inspect workspaces.`,
}

var workspaceListLocal bool

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List workspaces",
	Long: `List workspaces from the server or local storage.

By default, lists workspaces from the remote server.
Use --local to list workspaces stored in the central location.

Examples:
  # List all workspaces from server (default)
  nebi workspace list

  # List locally stored workspaces
  nebi workspace list --local`,
	Args: cobra.NoArgs,
	Run:  runWorkspaceList,
}

var workspaceListTagsCmd = &cobra.Command{
	Use:   "tags <workspace>",
	Short: "List tags for a workspace",
	Long: `List all published tags for a workspace.

Example:
  nebi workspace tags myworkspace`,
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
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
	workspaceCmd.AddCommand(workspaceInfoCmd)

	// workspace tags is a subcommand of list
	workspaceListCmd.AddCommand(workspaceListTagsCmd)

	// Add flags
	workspaceListCmd.Flags().BoolVar(&workspaceListLocal, "local", false, "List locally stored workspaces")
}

func runWorkspaceList(cmd *cobra.Command, args []string) {
	if workspaceListLocal {
		runWorkspaceListLocal()
		return
	}

	// Default: list from server
	client := mustGetClient()
	ctx := mustGetAuthContext()

	envs, err := client.ListEnvironments(ctx)
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
			owner = env.Owner.Username
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			env.Name,
			env.Status,
			env.PackageManager,
			owner,
		)
	}
	w.Flush()
}

func runWorkspaceListLocal() {
	envsDir, err := getEnvsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Load the index to map UUIDs back to workspace names
	index, err := loadIndex()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load index: %v\n", err)
		os.Exit(1)
	}

	// Build reverse map: UUID -> workspace name
	uuidToName := make(map[string]string)
	for name, uuid := range index.Workspaces {
		uuidToName[uuid] = name
	}

	// Check if envs directory exists
	entries, err := os.ReadDir(envsDir)
	if os.IsNotExist(err) {
		fmt.Println("No local workspaces found")
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to read envs directory: %v\n", err)
		os.Exit(1)
	}

	// Collect all local environments
	type localEnv struct {
		Workspace string
		Registry  string
		Tag       string
		Path      string
	}
	var localEnvs []localEnv

	for _, uuidEntry := range entries {
		if !uuidEntry.IsDir() || uuidEntry.Name() == "index.json" {
			continue
		}

		workspaceUUID := uuidEntry.Name()
		workspaceName := uuidToName[workspaceUUID]
		if workspaceName == "" {
			workspaceName = workspaceUUID[:8] + "..." // Show truncated UUID if name unknown
		}

		uuidPath := envsDir + "/" + uuidEntry.Name()
		registries, err := os.ReadDir(uuidPath)
		if err != nil {
			continue
		}

		for _, regEntry := range registries {
			if !regEntry.IsDir() {
				continue
			}

			regPath := uuidPath + "/" + regEntry.Name()
			tags, err := os.ReadDir(regPath)
			if err != nil {
				continue
			}

			for _, tagEntry := range tags {
				if !tagEntry.IsDir() {
					continue
				}

				tagPath := regPath + "/" + tagEntry.Name()
				// Only include if pixi.toml exists
				if envExistsLocally(tagPath) {
					localEnvs = append(localEnvs, localEnv{
						Workspace: workspaceName,
						Registry:  regEntry.Name(),
						Tag:       tagEntry.Name(),
						Path:      tagPath,
					})
				}
			}
		}
	}

	if len(localEnvs) == 0 {
		fmt.Println("No local workspaces found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKSPACE\tREGISTRY\tTAG\tPATH")
	for _, env := range localEnvs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			env.Workspace,
			env.Registry,
			env.Tag,
			env.Path,
		)
	}
	w.Flush()
}

func runWorkspaceListTags(cmd *cobra.Command, args []string) {
	workspaceName := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get publications (tags)
	pubs, err := client.GetEnvironmentPublications(ctx, env.ID)
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
		digest := pub.Digest
		if len(digest) > 19 {
			digest = digest[:19] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			pub.Tag,
			pub.RegistryName,
			pub.Repository,
			digest,
			pub.PublishedAt,
		)
	}
	w.Flush()
}

func runWorkspaceDelete(cmd *cobra.Command, args []string) {
	workspaceName := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Delete
	err = client.DeleteEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to delete workspace: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleted workspace %q\n", workspaceName)
}

func runWorkspaceInfo(cmd *cobra.Command, args []string) {
	workspaceName := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get full details
	envDetail, err := client.GetEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get workspace details: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Name:            %s\n", envDetail.Name)
	fmt.Printf("ID:              %s\n", envDetail.ID)
	fmt.Printf("Status:          %s\n", envDetail.Status)
	fmt.Printf("Package Manager: %s\n", envDetail.PackageManager)
	if envDetail.Owner != nil {
		fmt.Printf("Owner:           %s\n", envDetail.Owner.Username)
	}
	fmt.Printf("Size:            %d bytes\n", envDetail.SizeBytes)
	fmt.Printf("Created:         %s\n", envDetail.CreatedAt)
	fmt.Printf("Updated:         %s\n", envDetail.UpdatedAt)

	// Get packages
	packages, err := client.GetEnvironmentPackages(ctx, env.ID)
	if err == nil && len(packages) > 0 {
		fmt.Printf("\nPackages (%d):\n", len(packages))
		for _, pkg := range packages {
			fmt.Printf("  - %s", pkg.Name)
			if pkg.Version != "" {
				fmt.Printf(" (%s)", pkg.Version)
			}
			fmt.Println()
		}
	}
}

// findWorkspaceByName looks up a workspace by name and returns it.
func findWorkspaceByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Environment, error) {
	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %v", err)
	}

	for _, env := range envs {
		if env.Name == name {
			return &env, nil
		}
	}

	return nil, fmt.Errorf("workspace %q not found", name)
}
