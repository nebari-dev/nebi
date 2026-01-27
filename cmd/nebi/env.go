package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
	Long:  `List, delete, and inspect environments.`,
}

var envListLocal bool
var envListJSON bool

var envListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List environments",
	Long: `List environments from the server, or locally pulled environments.

Examples:
  # List all server environments
  nebi env list

  # List locally pulled environments with drift status
  nebi env list --local`,
	Args: cobra.NoArgs,
	Run:  runEnvList,
}

var envPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stale entries from local index",
	Long: `Remove entries from the local environment index where the directory
no longer exists on disk.

This cleans up the index after environments have been moved or deleted
outside of nebi. It does NOT delete any files.

Examples:
  # Remove stale entries
  nebi env prune`,
	Args: cobra.NoArgs,
	Run:  runEnvPrune,
}

var envVersionsCmd = &cobra.Command{
	Use:   "versions <env>",
	Short: "List versions for an environment",
	Long: `List all published versions for an environment.

Example:
  nebi env versions myenv`,
	Args: cobra.ExactArgs(1),
	Run:  runEnvVersions,
}

var envDeleteCmd = &cobra.Command{
	Use:     "delete <env>",
	Aliases: []string{"rm"},
	Short:   "Delete an environment",
	Long: `Delete an environment from the server.

Example:
  nebi env delete myenv`,
	Args: cobra.ExactArgs(1),
	Run:  runEnvDelete,
}

var envInfoPath string
var envInfoPackages bool
var envInfoJSON bool

var envInfoCmd = &cobra.Command{
	Use:   "info [<env>]",
	Short: "Show environment details",
	Long: `Show detailed information about an environment.

When run without arguments in a directory containing a .nebi metadata file,
shows both local drift status and server-side environment details.

When given an environment name, shows server-side details only.

Examples:
  # From an environment directory (reads .nebi to detect environment)
  nebi env info

  # Explicit environment name (server lookup only)
  nebi env info myenv

  # From a specific path
  nebi env info -C /path/to/env`,
	Args: cobra.MaximumNArgs(1),
	Run:  runEnvInfo,
}

var envDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show environment differences (alias for 'nebi diff')",
	Long:  `This is an alias for 'nebi diff'. See 'nebi diff --help' for full documentation.`,
	Args:  cobra.MaximumNArgs(2),
	Run:   runDiff,
}

func init() {
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envDeleteCmd)
	envCmd.AddCommand(envInfoCmd)
	envCmd.AddCommand(envDiffCmd)
	envCmd.AddCommand(envPruneCmd)
	envCmd.AddCommand(envVersionsCmd)

	// env info flags
	envInfoCmd.Flags().StringVarP(&envInfoPath, "path", "C", ".", "Environment directory path")
	envInfoCmd.Flags().BoolVarP(&envInfoPackages, "packages", "p", false, "Show package list")
	envInfoCmd.Flags().BoolVar(&envInfoJSON, "json", false, "Output as JSON")

	// env list flags
	envListCmd.Flags().BoolVar(&envListLocal, "local", false, "List locally pulled environments with drift status")
	envListCmd.Flags().BoolVar(&envListJSON, "json", false, "Output as JSON")

	// env diff mirrors the top-level diff flags
	envDiffCmd.Flags().BoolVar(&diffRemote, "remote", false, "Compare against current remote version")
	envDiffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output as JSON")
	envDiffCmd.Flags().BoolVar(&diffLock, "lock", false, "Show full lock file diff")
	envDiffCmd.Flags().BoolVar(&diffToml, "toml", false, "Show only pixi.toml diff")
	envDiffCmd.Flags().StringVarP(&diffPath, "path", "C", ".", "Environment directory path")
}

func runEnvList(cmd *cobra.Command, args []string) {
	if envListLocal {
		runEnvListLocal()
		return
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list environments: %v\n", err)
		osExit(1)
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

// envListEntry is the JSON output structure for env list --local --json.
type envListEntry struct {
	Env      string `json:"env"`
	Version  string `json:"version"`
	Status   string `json:"status"`
	Path     string `json:"path"`
	IsGlobal bool   `json:"is_global"`
}

// runEnvListLocal lists locally pulled environments with drift indicators.
func runEnvListLocal() {
	store := localindex.NewStore()
	index, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load local index: %v\n", err)
		osExit(1)
	}

	if len(index.Entries) == 0 {
		if envListJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No locally pulled environments found")
			fmt.Println("\nUse 'nebi pull <env>:<version>' to pull an environment.")
		}
		return
	}

	if envListJSON {
		var entries []envListEntry
		for _, entry := range index.Entries {
			status := getLocalEntryStatus(entry)
			entries = append(entries, envListEntry{
				Env:      entry.SpecName,
				Version:  entry.VersionName,
				Status:   status,
				Path:     entry.Path,
				IsGlobal: entry.IsGlobal(),
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
	fmt.Fprintln(w, "ENV\tVERSION\tSTATUS\tLOCATION")
	for _, entry := range index.Entries {
		status := getLocalEntryStatus(entry)
		if status == "missing" {
			hasMissing = true
		}

		location := formatLocation(entry.Path, entry.IsGlobal())
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			entry.SpecName,
			entry.VersionName,
			status,
			location,
		)
	}
	w.Flush()

	if hasMissing {
		fmt.Println("\nRun 'nebi env prune' to remove stale entries.")
	}
}

// getLocalEntryStatus checks the drift status of a local environment entry.
func getLocalEntryStatus(entry localindex.Entry) string {
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
// and shortening UUIDs in global environment paths.
func formatLocation(path string, isGlobal bool) string {
	home, _ := os.UserHomeDir()
	display := path
	if home != "" && strings.HasPrefix(path, home) {
		display = "~" + path[len(home):]
	}

	if isGlobal {
		// Abbreviate UUIDs in global environment paths for readability.
		// Global paths look like: ~/.local/share/nebi/repos/<uuid>/<version>
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

// runEnvPrune removes stale entries from the local index.
func runEnvPrune(cmd *cobra.Command, args []string) {
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
		fmt.Printf("  - %s:%s (%s)\n", entry.SpecName, entry.VersionName, entry.Path)
	}
}

func runEnvVersions(cmd *cobra.Command, args []string) {
	envName := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Get server-side versions (tags)
	tags, err := client.GetEnvironmentTags(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list versions: %v\n", err)
		osExit(1)
	}

	if len(tags) == 0 {
		fmt.Printf("No versions for %q\n", envName)
		fmt.Println("\nPush a version with: nebi push " + envName + ":<version>")
		return
	}

	// Check if there's a default version
	var defaultVersionNum *int
	if env.DefaultVersionID != nil {
		defaultVersionNum = env.DefaultVersionID
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tNUMBER\tDEFAULT")
	for _, tag := range tags {
		isDefault := ""
		if defaultVersionNum != nil && tag.VersionNumber == *defaultVersionNum {
			isDefault = "✓"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\n",
			tag.Tag,
			tag.VersionNumber,
			isDefault,
		)
	}
	w.Flush()
}

func runEnvDelete(cmd *cobra.Command, args []string) {
	envName := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Delete
	err = client.DeleteEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to delete environment: %v\n", err)
		osExit(1)
	}

	fmt.Printf("Deleted environment %q\n", envName)
}

func runEnvInfo(cmd *cobra.Command, args []string) {
	if len(args) == 1 {
		// Explicit environment name: server-only lookup (original behavior)
		runEnvInfoByName(args[0])
		return
	}

	// No argument: detect environment from .nebi file in current directory
	runEnvInfoFromCwd()
}

// runEnvInfoFromCwd shows combined local status and server info
// by reading the .nebi metadata file from the current (or -C) directory.
func runEnvInfoFromCwd() {
	dir := envInfoPath

	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Read .nebi metadata
	nf, err := nebifile.Read(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Not a nebi environment directory (no .nebi file found)\n")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Hint: Specify an environment name: nebi env info <name>")
		fmt.Fprintln(os.Stderr, "      Or pull an environment first: nebi pull <env>:<version>")
		osExit(1)
	}

	// Show local status section
	fmt.Println("Local:")
	fmt.Printf("  Name:        %s\n", nf.Origin.SpecName)
	fmt.Printf("  Version:     %s\n", nf.Origin.VersionName)
	if nf.Origin.VersionID != "" {
		fmt.Printf("  Version ID:  %s\n", nf.Origin.VersionID)
	}
	if nf.Origin.ServerURL != "" {
		fmt.Printf("  Server:      %s\n", nf.Origin.ServerURL)
	}

	// Perform drift check
	ws := drift.CheckWithNebiFile(absDir, nf)
	fmt.Printf("  Status:      %s\n", ws.Overall)
	for _, fs := range ws.Files {
		if fs.Status != drift.StatusClean {
			fmt.Printf("    %-12s %s\n", fs.Filename+":", string(fs.Status))
		}
	}

	// Show server info section
	fmt.Println("")
	client := mustGetClient()
	ctx := mustGetAuthContext()

	env, err := findEnvByName(client, ctx, nf.Origin.SpecName)
	if err != nil {
		fmt.Println("Server:")
		fmt.Printf("  (environment %q not found on server)\n", nf.Origin.SpecName)
		return
	}

	envDetail, err := client.GetEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Println("Server:")
		fmt.Printf("  (failed to get environment details: %v)\n", err)
		return
	}

	// Get packages (always fetch for count)
	packages, _ := client.GetEnvironmentPackages(ctx, env.ID)

	printServerInfo(envDetail, len(packages))

	// Show package details only if flag is set
	if envInfoPackages && len(packages) > 0 {
		fmt.Printf("\n  Package list:\n")
		for _, pkg := range packages {
			fmt.Printf("    - %s", pkg.Name)
			if pkg.Version != "" {
				fmt.Printf(" (%s)", pkg.Version)
			}
			fmt.Println()
		}
	}
}

// runEnvInfoByName shows server-side environment info by name.
func runEnvInfoByName(envName string) {
	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment by name
	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Get full details
	envDetail, err := client.GetEnvironment(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get environment details: %v\n", err)
		osExit(1)
	}

	// Get packages (always fetch for count, show details only with --packages)
	packages, _ := client.GetEnvironmentPackages(ctx, env.ID)

	// JSON output
	if envInfoJSON {
		// Only include package details in JSON if --packages flag is set
		if !envInfoPackages {
			outputEnvInfoJSON(envDetail, nil, len(packages))
		} else {
			outputEnvInfoJSON(envDetail, packages, len(packages))
		}
		return
	}

	fmt.Printf("Name:            %s\n", envDetail.Name)
	fmt.Printf("Spec ID:         %s\n", envDetail.ID)
	fmt.Printf("Status:          %s\n", envDetail.Status)
	fmt.Printf("Package Manager: %s\n", envDetail.PackageManager)
	if envDetail.Owner != nil {
		fmt.Printf("Owner:           %s\n", envDetail.Owner.Username)
	}
	fmt.Printf("Size:            %s\n", formatBytes(envDetail.SizeBytes))
	fmt.Printf("Packages:        %d\n", len(packages))
	fmt.Printf("Created:         %s\n", envDetail.CreatedAt)
	fmt.Printf("Updated:         %s\n", envDetail.UpdatedAt)

	// Show package details only if flag is set
	if envInfoPackages && len(packages) > 0 {
		fmt.Printf("\nPackage list:\n")
		for _, pkg := range packages {
			fmt.Printf("  - %s", pkg.Name)
			if pkg.Version != "" {
				fmt.Printf(" (%s)", pkg.Version)
			}
			fmt.Println()
		}
	}
}

// outputEnvInfoJSON outputs environment info as JSON.
func outputEnvInfoJSON(envDetail *cliclient.Environment, packages []cliclient.Package, packageCount int) {
	type pkgJSON struct {
		Name    string `json:"name"`
		Version string `json:"version,omitempty"`
	}

	type ownerJSON struct {
		Username string `json:"username"`
	}

	type envInfoJSON struct {
		Name           string     `json:"name"`
		ID             string     `json:"id"`
		Status         string     `json:"status"`
		PackageManager string     `json:"package_manager"`
		Owner          *ownerJSON `json:"owner,omitempty"`
		SizeBytes      int64      `json:"size_bytes"`
		PackageCount   int        `json:"package_count"`
		CreatedAt      string     `json:"created_at"`
		UpdatedAt      string     `json:"updated_at"`
		Packages       []pkgJSON  `json:"packages,omitempty"`
	}

	output := envInfoJSON{
		Name:           envDetail.Name,
		ID:             envDetail.ID,
		Status:         envDetail.Status,
		PackageManager: envDetail.PackageManager,
		SizeBytes:      envDetail.SizeBytes,
		PackageCount:   packageCount,
		CreatedAt:      envDetail.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      envDetail.UpdatedAt.Format(time.RFC3339),
	}

	if envDetail.Owner != nil {
		output.Owner = &ownerJSON{Username: envDetail.Owner.Username}
	}

	if len(packages) > 0 {
		output.Packages = make([]pkgJSON, len(packages))
		for i, pkg := range packages {
			output.Packages[i] = pkgJSON{Name: pkg.Name, Version: pkg.Version}
		}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to marshal JSON: %v\n", err)
		osExit(1)
	}
	fmt.Println(string(data))
}

// printServerInfo prints the server details section for environment info.
func printServerInfo(envDetail *cliclient.Environment, packageCount int) {
	fmt.Println("Server:")
	fmt.Printf("  Name:            %s\n", envDetail.Name)
	fmt.Printf("  Spec ID:         %s\n", envDetail.ID)
	fmt.Printf("  Status:          %s\n", envDetail.Status)
	fmt.Printf("  Package Manager: %s\n", envDetail.PackageManager)
	if envDetail.Owner != nil {
		fmt.Printf("  Owner:           %s\n", envDetail.Owner.Username)
	}
	fmt.Printf("  Size:            %s\n", formatBytes(envDetail.SizeBytes))
	fmt.Printf("  Packages:        %d\n", packageCount)
	fmt.Printf("  Created:         %s\n", envDetail.CreatedAt)
	fmt.Printf("  Updated:         %s\n", envDetail.UpdatedAt)
}

// findEnvByName looks up an environment by name and returns it.
func findEnvByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Environment, error) {
	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %v", err)
	}

	for _, env := range envs {
		if env.Name == name {
			return &env, nil
		}
	}

	return nil, fmt.Errorf("environment %q not found", name)
}
