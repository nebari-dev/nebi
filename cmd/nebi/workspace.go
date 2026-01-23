package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage workspaces",
	Long:    `List, delete, and inspect workspaces.`,
}

var workspaceListLocal bool
var workspaceListJSON bool

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

var workspaceInfoPath string

var workspaceInfoCmd = &cobra.Command{
	Use:   "info [<workspace>]",
	Short: "Show workspace details",
	Long: `Show detailed information about a workspace.

When run without arguments in a directory containing a .nebi metadata file,
shows both local drift status and server-side workspace details.

When given a workspace name, shows server-side details only.

Examples:
  # From a workspace directory (reads .nebi to detect workspace)
  nebi workspace info

  # Explicit workspace name (server lookup only)
  nebi workspace info myworkspace

  # From a specific path
  nebi workspace info -C /path/to/workspace`,
	Args: cobra.MaximumNArgs(1),
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

	// workspace info flags
	workspaceInfoCmd.Flags().StringVarP(&workspaceInfoPath, "path", "C", ".", "Workspace directory path")

	// workspace list flags
	workspaceListCmd.Flags().BoolVar(&workspaceListLocal, "local", false, "List locally pulled workspaces with drift status")
	workspaceListCmd.Flags().BoolVar(&workspaceListJSON, "json", false, "Output as JSON")

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

// workspaceListEntry is the JSON output structure for workspace list --local --json.
type workspaceListEntry struct {
	Workspace string `json:"workspace"`
	Tag       string `json:"tag"`
	Status    string `json:"status"`
	Path      string `json:"path"`
	IsGlobal  bool   `json:"is_global"`
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
		if workspaceListJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No locally pulled workspaces found")
			fmt.Println("\nUse 'nebi pull <workspace>:<tag>' to pull a workspace.")
		}
		return
	}

	if workspaceListJSON {
		var entries []workspaceListEntry
		for _, entry := range index.Workspaces {
			status := getLocalEntryStatus(entry)
			entries = append(entries, workspaceListEntry{
				Workspace: entry.Workspace,
				Tag:       entry.Tag,
				Status:    status,
				Path:      entry.Path,
				IsGlobal:  entry.IsGlobal,
			})
		}
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to encode JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	hasMissing := false

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKSPACE\tTAG\tSTATUS\tLOCATION")
	for _, entry := range index.Workspaces {
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

// formatLocation formats a path for display, abbreviating home directory
// and shortening UUIDs in global workspace paths.
func formatLocation(path string, isGlobal bool) string {
	home, _ := os.UserHomeDir()
	display := path
	if home != "" && strings.HasPrefix(path, home) {
		display = "~" + path[len(home):]
	}

	if isGlobal {
		// Abbreviate UUIDs in global workspace paths for readability.
		// Global paths look like: ~/.local/share/nebi/workspaces/<uuid>/<tag>
		display = abbreviateUUID(display)
		return display + " (global)"
	}

	return display + " (local)"
}

// abbreviateUUID shortens UUID path components (32 hex + 4 hyphens = 36 chars)
// to their first 8 characters for display.
func abbreviateUUID(path string) string {
	parts := strings.Split(path, string(filepath.Separator))
	for i, part := range parts {
		if isUUID(part) {
			parts[i] = part[:8]
		}
	}
	return strings.Join(parts, string(filepath.Separator))
}

// isUUID checks if a string looks like a UUID (8-4-4-4-12 hex pattern, 36 chars).
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
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
	if len(args) == 1 {
		// Explicit workspace name: server-only lookup (original behavior)
		runWorkspaceInfoByName(args[0])
		return
	}

	// No argument: detect workspace from .nebi file in current directory
	runWorkspaceInfoFromCwd()
}

// runWorkspaceInfoFromCwd shows combined local status and server info
// by reading the .nebi metadata file from the current (or -C) directory.
func runWorkspaceInfoFromCwd() {
	dir := workspaceInfoPath

	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Read .nebi metadata
	nf, err := nebifile.Read(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Not a nebi workspace directory (no .nebi file found)\n")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Hint: Specify a workspace name: nebi workspace info <name>")
		fmt.Fprintln(os.Stderr, "      Or pull a workspace first: nebi pull <workspace>:<tag>")
		os.Exit(1)
	}

	// Show local status section
	fmt.Println("Local:")
	fmt.Printf("  Workspace: %s:%s\n", nf.Origin.Workspace, nf.Origin.Tag)
	if nf.Origin.Registry != "" {
		fmt.Printf("  Registry:  %s\n", nf.Origin.Registry)
	}
	if nf.Origin.ServerURL != "" {
		fmt.Printf("  Server:    %s\n", nf.Origin.ServerURL)
	}
	fmt.Printf("  Pulled:    %s (%s)\n", nf.Origin.PulledAt.Format("2006-01-02 15:04:05"), formatTimeAgo(nf.Origin.PulledAt))
	if nf.Origin.ManifestDigest != "" {
		fmt.Printf("  Digest:    %s\n", nf.Origin.ManifestDigest)
	}

	// Perform drift check
	ws := drift.CheckWithNebiFile(absDir, nf)
	fmt.Printf("  Status:    %s\n", ws.Overall)
	for _, fs := range ws.Files {
		if fs.Status != drift.StatusClean {
			fmt.Printf("    %-12s %s\n", fs.Filename+":", string(fs.Status))
		}
	}

	// Show server info section
	fmt.Println("")
	client := mustGetClient()
	ctx := mustGetAuthContext()

	env, err := findWorkspaceByName(client, ctx, nf.Origin.Workspace)
	if err != nil {
		fmt.Println("Server:")
		fmt.Printf("  (workspace %q not found on server)\n", nf.Origin.Workspace)
		return
	}

	envDetail, err := client.GetEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Println("Server:")
		fmt.Printf("  (failed to get details: %v)\n", err)
		return
	}

	printServerInfo(envDetail)

	// Get packages
	packages, err := client.GetEnvironmentPackages(ctx, env.ID)
	if err == nil && len(packages) > 0 {
		fmt.Printf("\n  Packages (%d):\n", len(packages))
		for _, pkg := range packages {
			fmt.Printf("    - %s", pkg.Name)
			if pkg.Version != "" {
				fmt.Printf(" (%s)", pkg.Version)
			}
			fmt.Println()
		}
	}
}

// runWorkspaceInfoByName shows server-side workspace info by name.
func runWorkspaceInfoByName(workspaceName string) {
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

// printServerInfo prints the server details section for workspace info.
func printServerInfo(envDetail *cliclient.Environment) {
	fmt.Println("Server:")
	fmt.Printf("  Name:            %s\n", envDetail.Name)
	fmt.Printf("  ID:              %s\n", envDetail.ID)
	fmt.Printf("  Status:          %s\n", envDetail.Status)
	fmt.Printf("  Package Manager: %s\n", envDetail.PackageManager)
	if envDetail.Owner != nil {
		fmt.Printf("  Owner:           %s\n", envDetail.Owner.Username)
	}
	fmt.Printf("  Size:            %d bytes\n", envDetail.SizeBytes)
	fmt.Printf("  Created:         %s\n", envDetail.CreatedAt)
	fmt.Printf("  Updated:         %s\n", envDetail.UpdatedAt)
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
