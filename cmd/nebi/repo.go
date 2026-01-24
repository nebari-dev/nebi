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

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage repos",
	Long:  `List, delete, and inspect repos.`,
}

var repoListLocal bool
var repoListJSON bool

var repoListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List repos",
	Long: `List repos from the server, or locally pulled repos.

Examples:
  # List all server repos
  nebi repo list

  # List locally pulled repos with drift status
  nebi repo list --local`,
	Args: cobra.NoArgs,
	Run:  runRepoList,
}

var repoPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stale entries from local index",
	Long: `Remove entries from the local repo index where the directory
no longer exists on disk.

This cleans up the index after repos have been moved or deleted
outside of nebi. It does NOT delete any files.

Examples:
  # Remove stale entries
  nebi repo prune`,
	Args: cobra.NoArgs,
	Run:  runRepoPrune,
}

var repoTagsCmd = &cobra.Command{
	Use:   "tags <repo>",
	Short: "List tags for a repo",
	Long: `List all published tags for a repo.

Example:
  nebi repo tags myrepo`,
	Args: cobra.ExactArgs(1),
	Run:  runRepoTags,
}

var repoDeleteCmd = &cobra.Command{
	Use:     "delete <repo>",
	Aliases: []string{"rm"},
	Short:   "Delete a repo",
	Long: `Delete a repo from the server.

Example:
  nebi repo delete myrepo`,
	Args: cobra.ExactArgs(1),
	Run:  runRepoDelete,
}

var repoInfoPath string

var repoInfoCmd = &cobra.Command{
	Use:   "info [<repo>]",
	Short: "Show repo details",
	Long: `Show detailed information about a repo.

When run without arguments in a directory containing a .nebi metadata file,
shows both local drift status and server-side repo details.

When given a repo name, shows server-side details only.

Examples:
  # From a repo directory (reads .nebi to detect repo)
  nebi repo info

  # Explicit repo name (server lookup only)
  nebi repo info myrepo

  # From a specific path
  nebi repo info -C /path/to/repo`,
	Args: cobra.MaximumNArgs(1),
	Run:  runRepoInfo,
}

var repoDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show repo differences (alias for 'nebi diff')",
	Long:  `This is an alias for 'nebi diff'. See 'nebi diff --help' for full documentation.`,
	Args:  cobra.MaximumNArgs(2),
	Run:   runDiff,
}

func init() {
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoDeleteCmd)
	repoCmd.AddCommand(repoInfoCmd)
	repoCmd.AddCommand(repoDiffCmd)
	repoCmd.AddCommand(repoPruneCmd)
	repoCmd.AddCommand(repoTagsCmd)

	// repo info flags
	repoInfoCmd.Flags().StringVarP(&repoInfoPath, "path", "C", ".", "Repo directory path")

	// repo list flags
	repoListCmd.Flags().BoolVar(&repoListLocal, "local", false, "List locally pulled repos with drift status")
	repoListCmd.Flags().BoolVar(&repoListJSON, "json", false, "Output as JSON")

	// repo diff mirrors the top-level diff flags
	repoDiffCmd.Flags().BoolVar(&diffRemote, "remote", false, "Compare against current remote tag")
	repoDiffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output as JSON")
	repoDiffCmd.Flags().BoolVar(&diffLock, "lock", false, "Show full lock file diff")
	repoDiffCmd.Flags().BoolVar(&diffToml, "toml", false, "Show only pixi.toml diff")
	repoDiffCmd.Flags().StringVarP(&diffPath, "path", "C", ".", "Repo directory path")
}

func runRepoList(cmd *cobra.Command, args []string) {
	if repoListLocal {
		runRepoListLocal()
		return
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list repos: %v\n", err)
		osExit(1)
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

// repoListEntry is the JSON output structure for repo list --local --json.
type repoListEntry struct {
	Repo     string `json:"repo"`
	Tag      string `json:"tag"`
	Status   string `json:"status"`
	Path     string `json:"path"`
	IsGlobal bool   `json:"is_global"`
}

// runRepoListLocal lists locally pulled repos with drift indicators.
func runRepoListLocal() {
	store := localindex.NewStore()
	index, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load local index: %v\n", err)
		osExit(1)
	}

	if len(index.Repos) == 0 {
		if repoListJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No locally pulled repos found")
			fmt.Println("\nUse 'nebi pull <repo>:<tag>' to pull a repo.")
		}
		return
	}

	if repoListJSON {
		var entries []repoListEntry
		for _, entry := range index.Repos {
			status := getLocalEntryStatus(entry)
			entries = append(entries, repoListEntry{
				Repo:     entry.Repo,
				Tag:      entry.Tag,
				Status:   status,
				Path:     entry.Path,
				IsGlobal: entry.IsGlobal,
			})
		}
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to encode JSON: %v\n", err)
			osExit(1)
		}
		fmt.Println(string(data))
		return
	}

	hasMissing := false

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\tTAG\tSTATUS\tLOCATION")
	for _, entry := range index.Repos {
		status := getLocalEntryStatus(entry)
		if status == "missing" {
			hasMissing = true
		}

		location := formatLocation(entry.Path, entry.IsGlobal)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			entry.Repo,
			entry.Tag,
			status,
			location,
		)
	}
	w.Flush()

	if hasMissing {
		fmt.Println("\nRun 'nebi repo prune' to remove stale entries.")
	}
}

// getLocalEntryStatus checks the drift status of a local repo entry.
func getLocalEntryStatus(entry localindex.RepoEntry) string {
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
// and shortening UUIDs in global repo paths.
func formatLocation(path string, isGlobal bool) string {
	home, _ := os.UserHomeDir()
	display := path
	if home != "" && strings.HasPrefix(path, home) {
		display = "~" + path[len(home):]
	}

	if isGlobal {
		// Abbreviate UUIDs in global repo paths for readability.
		// Global paths look like: ~/.local/share/nebi/repos/<uuid>/<tag>
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

// runRepoPrune removes stale entries from the local index.
func runRepoPrune(cmd *cobra.Command, args []string) {
	store := localindex.NewStore()

	removed, err := store.Prune()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to prune local index: %v\n", err)
		osExit(1)
	}

	if len(removed) == 0 {
		fmt.Println("No stale entries found")
		return
	}

	fmt.Printf("Removed %d stale entries:\n", len(removed))
	for _, entry := range removed {
		fmt.Printf("  - %s:%s (%s)\n", entry.Repo, entry.Tag, entry.Path)
	}
}

func runRepoTags(cmd *cobra.Command, args []string) {
	repoName := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find repo by name
	env, err := findRepoByName(client, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Get publications (tags)
	pubs, err := client.GetEnvironmentPublications(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list tags: %v\n", err)
		osExit(1)
	}

	if len(pubs) == 0 {
		fmt.Printf("No published tags for %q\n", repoName)
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

func runRepoDelete(cmd *cobra.Command, args []string) {
	repoName := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find repo by name
	env, err := findRepoByName(client, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Delete
	err = client.DeleteEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to delete repo: %v\n", err)
		osExit(1)
	}

	fmt.Printf("Deleted repo %q\n", repoName)
}

func runRepoInfo(cmd *cobra.Command, args []string) {
	if len(args) == 1 {
		// Explicit repo name: server-only lookup (original behavior)
		runRepoInfoByName(args[0])
		return
	}

	// No argument: detect repo from .nebi file in current directory
	runRepoInfoFromCwd()
}

// runRepoInfoFromCwd shows combined local status and server info
// by reading the .nebi metadata file from the current (or -C) directory.
func runRepoInfoFromCwd() {
	dir := repoInfoPath

	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Read .nebi metadata
	nf, err := nebifile.Read(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Not a nebi repo directory (no .nebi file found)\n")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Hint: Specify a repo name: nebi repo info <name>")
		fmt.Fprintln(os.Stderr, "      Or pull a repo first: nebi pull <repo>:<tag>")
		osExit(1)
	}

	// Show local status section
	fmt.Println("Local:")
	fmt.Printf("  Repo: %s:%s\n", nf.Origin.Repo, nf.Origin.Tag)
	if nf.Origin.RegistryURL != "" {
		fmt.Printf("  Registry:  %s\n", nf.Origin.RegistryURL)
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

	env, err := findRepoByName(client, ctx, nf.Origin.Repo)
	if err != nil {
		fmt.Println("Server:")
		fmt.Printf("  (repo %q not found on server)\n", nf.Origin.Repo)
		return
	}

	envDetail, err := client.GetEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Println("Server:")
		fmt.Printf("  (failed to get repo details: %v)\n", err)
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

// runRepoInfoByName shows server-side repo info by name.
func runRepoInfoByName(repoName string) {
	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find repo by name
	env, err := findRepoByName(client, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Get full details
	envDetail, err := client.GetEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get repo details: %v\n", err)
		osExit(1)
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

// printServerInfo prints the server details section for repo info.
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

// findRepoByName looks up a repo by name and returns it.
func findRepoByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Environment, error) {
	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %v", err)
	}

	for _, env := range envs {
		if env.Name == name {
			return &env, nil
		}
	}

	return nil, fmt.Errorf("repo %q not found", name)
}
