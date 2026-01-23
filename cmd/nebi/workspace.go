package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/localindex"
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
	Long: `List workspaces from the server, or locally pulled workspaces.

Examples:
  # List all server workspaces
  nebi workspace list

  # List locally pulled workspaces with drift status
  nebi workspace list --local`,
	Args: cobra.NoArgs,
	Run:  runWorkspaceList,
}

var workspacePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stale entries from local index",
	Long: `Remove entries from the local workspace index where the directory
no longer exists on disk.

This cleans up the index after workspaces have been moved or deleted
outside of nebi. It does NOT delete any files.

Examples:
  # Remove stale entries
  nebi workspace prune`,
	Args: cobra.NoArgs,
	Run:  runWorkspacePrune,
}

var workspaceTagsCmd = &cobra.Command{
	Use:   "tags <workspace>",
	Short: "List tags for a workspace",
	Long: `List all published tags for a workspace.

Example:
  nebi workspace tags myworkspace
  nebi ws tags myworkspace`,
	Args: cobra.ExactArgs(1),
	Run:  runWorkspaceTags,
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

var workspaceDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show workspace differences (alias for 'nebi diff')",
	Long:  `This is an alias for 'nebi diff'. See 'nebi diff --help' for full documentation.`,
	Args:  cobra.MaximumNArgs(2),
	Run:   runDiff,
}

func init() {
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
	workspaceCmd.AddCommand(workspaceInfoCmd)
	workspaceCmd.AddCommand(workspaceDiffCmd)
	workspaceCmd.AddCommand(workspacePruneCmd)
	workspaceCmd.AddCommand(workspaceTagsCmd)

	// workspace list flags
	workspaceListCmd.Flags().BoolVar(&workspaceListLocal, "local", false, "List locally pulled workspaces with drift status")

	// workspace diff mirrors the top-level diff flags
	workspaceDiffCmd.Flags().BoolVar(&diffRemote, "remote", false, "Compare against current remote tag")
	workspaceDiffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output as JSON")
	workspaceDiffCmd.Flags().BoolVar(&diffLock, "lock", false, "Show full lock file diff")
	workspaceDiffCmd.Flags().BoolVar(&diffToml, "toml", false, "Show only pixi.toml diff")
	workspaceDiffCmd.Flags().StringVarP(&diffPath, "path", "C", ".", "Workspace directory path")
}

func runWorkspaceList(cmd *cobra.Command, args []string) {
	if workspaceListLocal {
		runWorkspaceListLocal()
		return
	}

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

// runWorkspaceListLocal lists locally pulled workspaces with drift indicators.
func runWorkspaceListLocal() {
	store := localindex.NewStore()
	index, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load local index: %v\n", err)
		os.Exit(1)
	}

	if len(index.Workspaces) == 0 {
		fmt.Println("No locally pulled workspaces found")
		fmt.Println("\nUse 'nebi pull <workspace>:<tag>' to pull a workspace.")
		return
	}

	hasMissing := false

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKSPACE\tTAG\tSTATUS\tLOCATION")
	for _, entry := range index.Workspaces {
		// Check if path exists
		status := getLocalEntryStatus(entry)
		if status == "missing" {
			hasMissing = true
		}

		location := formatLocation(entry.Path, entry.IsGlobal)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			entry.Workspace,
			entry.Tag,
			status,
			location,
		)
	}
	w.Flush()

	if hasMissing {
		fmt.Println("\nRun 'nebi workspace prune' to remove stale entries.")
	}
}

// getLocalEntryStatus checks the drift status of a local workspace entry.
func getLocalEntryStatus(entry localindex.WorkspaceEntry) string {
	// Check if path exists
	if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
		return "missing"
	}

	// Check drift
	ws, err := drift.Check(entry.Path)
	if err != nil {
		return "unknown"
	}

	return string(ws.Overall)
}

// formatLocation formats a path for display, abbreviating home directory and global paths.
func formatLocation(path string, isGlobal bool) string {
	home, _ := os.UserHomeDir()
	display := path
	if home != "" && strings.HasPrefix(path, home) {
		display = "~" + path[len(home):]
	}

	// Truncate long global paths
	if isGlobal {
		dataDir := filepath.Join(home, ".local", "share", "nebi")
		if strings.HasPrefix(path, dataDir) {
			display = "~/.local/share/nebi/..." + " (global)"
			return display
		}
		return display + " (global)"
	}

	return display + " (local)"
}

// runWorkspacePrune removes stale entries from the local index.
func runWorkspacePrune(cmd *cobra.Command, args []string) {
	store := localindex.NewStore()

	removed, err := store.Prune()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to prune local index: %v\n", err)
		os.Exit(1)
	}

	if len(removed) == 0 {
		fmt.Println("No stale entries found")
		return
	}

	fmt.Printf("Removed %d stale entries:\n", len(removed))
	for _, entry := range removed {
		fmt.Printf("  - %s:%s (%s)\n", entry.Workspace, entry.Tag, entry.Path)
	}
}

func runWorkspaceTags(cmd *cobra.Command, args []string) {
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
