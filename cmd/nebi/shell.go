package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	shellPixiEnv string
	shellGlobal  bool
	shellLocal   bool
	shellPath    string
)

var shellCmd = &cobra.Command{
	Use:   "shell [<repo>[:<tag>]]",
	Short: "Activate repo shell",
	Long: `Activate a repo shell using pixi shell.

When run without arguments in a directory with .nebi metadata, uses the
local repo. When given a repo reference, looks up the local
index (preferring global copies) and falls back to pulling from server.

When multiple local copies exist, an interactive prompt lets you choose
which one to activate. Use -C to specify a path directly, or --global/--local
to filter by storage type.

Drift detection warns if local files have been modified since pull.

Examples:
  # Shell from current directory (reads .nebi metadata)
  nebi shell

  # Shell into specific repo by name
  nebi shell myrepo:v1.0.0

  # Shell into specific pixi environment
  nebi shell myrepo:v1.0.0 -e dev

  # Use global copy explicitly
  nebi shell myrepo:v1.0.0 --global

  # Use a local copy (prompts if multiple)
  nebi shell myrepo:v1.0.0 --local

  # Use repo at a specific path
  nebi shell myrepo:v1.0.0 -C ~/project-a`,
	Args: cobra.MaximumNArgs(1),
	Run:  runShell,
}

func init() {
	shellCmd.Flags().StringVarP(&shellPixiEnv, "env", "e", "", "Pixi environment name")
	shellCmd.Flags().BoolVarP(&shellGlobal, "global", "g", false, "Use global copy")
	shellCmd.Flags().BoolVarP(&shellLocal, "local", "l", false, "Use local copy (prompts if multiple)")
	shellCmd.Flags().StringVarP(&shellPath, "path", "C", "", "Use repo at specific directory path")
}

func runShell(cmd *cobra.Command, args []string) {
	// Validate flag conflicts
	if shellGlobal && shellLocal {
		fmt.Fprintf(os.Stderr, "Error: --global and --local are mutually exclusive\n")
		osExit(1)
	}
	if shellPath != "" && (shellGlobal || shellLocal) {
		fmt.Fprintf(os.Stderr, "Error: -C/--path cannot be combined with --global or --local\n")
		osExit(1)
	}

	var shellDir string

	if shellPath != "" {
		// Explicit path - resolve and validate
		shellDir = resolveShellFromPath(shellPath)
	} else if len(args) == 0 {
		// No argument - use current directory
		shellDir = resolveShellFromCwd()
	} else {
		// Parse repo:tag format
		repoName, tag, err := parseRepoRef(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			osExit(1)
		}
		shellDir = resolveShellFromRef(repoName, tag)
	}

	// Check for drift and warn
	checkShellDrift(shellDir)

	// Run pixi shell
	execPixiShell(shellDir, shellPixiEnv)
}

// resolveShellFromPath resolves a shell directory from an explicit path.
func resolveShellFromPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Check the directory exists
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: No nebi repo found at %s\n", absPath)
		osExit(1)
	}

	// Check for .nebi metadata or pixi.toml
	if nebifile.Exists(absPath) {
		return absPath
	}
	if _, err := os.Stat(filepath.Join(absPath, "pixi.toml")); err == nil {
		return absPath
	}

	fmt.Fprintf(os.Stderr, "Error: No nebi repo found at %s\n", absPath)
	osExit(1)
	return ""
}

// resolveShellFromCwd resolves a shell directory from the current working directory.
func resolveShellFromCwd() string {
	absDir, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Check for .nebi metadata
	if nebifile.Exists(absDir) {
		return absDir
	}

	// Check for pixi.toml (can still shell without .nebi)
	if _, err := os.Stat(filepath.Join(absDir, "pixi.toml")); err == nil {
		return absDir
	}

	fmt.Fprintf(os.Stderr, "Error: No repo found in current directory\n")
	fmt.Fprintln(os.Stderr, "Run 'nebi pull <repo>:<tag>' to pull a repo, or specify one: 'nebi shell <repo>:<tag>'")
	osExit(1)
	return ""
}

// resolveShellFromRef resolves a shell directory from a repo reference.
// Priority depends on flags:
//   - --global: use global copy only
//   - --local: use local copies only (interactive select if multiple)
//   - default: global > single local > interactive select > pull from server
func resolveShellFromRef(repoName, tag string) string {
	store := localindex.NewStore()
	refStr := repoName
	if tag != "" {
		refStr += ":" + tag
	}

	// --global flag: force global copy
	if shellGlobal {
		if tag == "" {
			fmt.Fprintf(os.Stderr, "Error: --global requires a tag (e.g., %s:v1.0)\n", repoName)
			osExit(1)
		}
		global, err := store.FindGlobal(repoName, tag)
		if err != nil || global == nil {
			fmt.Fprintf(os.Stderr, "Error: No global copy of %s\n", refStr)
			fmt.Fprintf(os.Stderr, "Use 'nebi pull --global %s' to create one.\n", refStr)
			osExit(1)
		}
		if _, err := os.Stat(global.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Global copy of %s no longer exists at %s\n", refStr, global.Path)
			osExit(1)
		}
		fmt.Printf("Using global copy of %s\n", refStr)
		return global.Path
	}

	// --local flag: force local copies only
	if shellLocal {
		if tag == "" {
			fmt.Fprintf(os.Stderr, "Error: --local requires a tag (e.g., %s:v1.0)\n", repoName)
			osExit(1)
		}
		locals := findValidLocalCopies(store, repoName, tag)
		if len(locals) == 0 {
			fmt.Fprintf(os.Stderr, "Error: No local copies of %s found\n", refStr)
			osExit(1)
		}
		if len(locals) == 1 {
			fmt.Printf("Using local copy at %s\n", locals[0].Path)
			return locals[0].Path
		}
		return promptSelectCopy(locals, refStr)
	}

	// Default resolution: global > local > pull
	if tag != "" {
		// Check for global copy first (global always wins)
		global, err := store.FindGlobal(repoName, tag)
		if err == nil && global != nil {
			if _, err := os.Stat(global.Path); err == nil {
				fmt.Printf("Using global copy of %s\n", refStr)
				return global.Path
			}
		}

		// Check local copies
		locals := findValidLocalCopies(store, repoName, tag)
		if len(locals) == 1 {
			fmt.Printf("Using local copy at %s\n", locals[0].Path)
			return locals[0].Path
		}
		if len(locals) > 1 {
			return promptSelectCopy(locals, refStr)
		}
	}

	// Not in local index - pull from server
	return pullForShell(repoName, tag)
}

// findValidLocalCopies returns local (non-global) copies that still exist on disk.
func findValidLocalCopies(store *localindex.Store, repo, tag string) []localindex.RepoEntry {
	matches, err := store.FindByRepoTag(repo, tag)
	if err != nil {
		return nil
	}

	var valid []localindex.RepoEntry
	for _, m := range matches {
		if m.IsGlobal() {
			continue
		}
		if _, err := os.Stat(m.Path); err == nil {
			valid = append(valid, m)
		}
	}
	return valid
}

// promptSelectCopy presents an interactive selection prompt for multiple copies.
// In non-interactive mode (no TTY), prints an error with available options and exits.
func promptSelectCopy(copies []localindex.RepoEntry, refStr string) string {
	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Error: Multiple local copies of %s found, cannot disambiguate without a TTY.\n\n", refStr)
		fmt.Fprintf(os.Stderr, "Available copies:\n")
		for _, c := range copies {
			fmt.Fprintf(os.Stderr, "  %s  (pulled %s)\n", shortenPath(c.Path), formatTimeAgo(c.PulledAt))
		}
		fmt.Fprintf(os.Stderr, "\nUse -C to specify:\n")
		for _, c := range copies {
			fmt.Fprintf(os.Stderr, "  nebi shell %s -C %s\n", refStr, c.Path)
		}
		osExit(2)
	}

	// Interactive selection
	fmt.Printf("\nMultiple local copies found for %s:\n", refStr)
	for i, c := range copies {
		status := getDriftStatus(c.Path)
		fmt.Printf("  [%d] %s  (pulled %s, %s)\n", i+1, shortenPath(c.Path), formatTimeAgo(c.PulledAt), status)
	}
	fmt.Printf("\nSelect [1-%d] or use -C to specify: ", len(copies))

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		fmt.Fprintf(os.Stderr, "\nError: No selection made\n")
		osExit(1)
	}

	input := strings.TrimSpace(scanner.Text())
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(copies) {
		fmt.Fprintf(os.Stderr, "Error: Invalid selection %q\n", input)
		osExit(1)
	}

	selected := copies[choice-1]
	fmt.Printf("Using local copy at %s\n", shortenPath(selected.Path))
	return selected.Path
}

// getDriftStatus returns a short drift status string for display.
func getDriftStatus(dir string) string {
	ws, err := drift.Check(dir)
	if err != nil {
		return "unknown"
	}
	return string(ws.Overall)
}

// shortenPath replaces the home directory prefix with ~.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// pullForShell pulls a repo from the server for shell activation.
func pullForShell(repoName, tag string) string {
	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find repo by name
	env, err := findRepoByName(client, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Get versions to find the right one
	versions, err := client.GetEnvironmentVersions(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
		osExit(1)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Repo %q has no versions\n", repoName)
		osExit(1)
	}

	// Use the latest version
	latestVersion := versions[0]
	for _, v := range versions {
		if v.VersionNumber > latestVersion.VersionNumber {
			latestVersion = v
		}
	}

	// Resolve tag for display
	if tag == "" {
		tag = "latest"
	}

	refStr := repoName + ":" + tag
	fmt.Printf("Pulling %s (version %d)...\n", refStr, latestVersion.VersionNumber)

	// Get content
	pixiToml, err := client.GetVersionPixiToml(ctx, env.ID, latestVersion.VersionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.toml: %v\n", err)
		osExit(1)
	}
	pixiLock, err := client.GetVersionPixiLock(ctx, env.ID, latestVersion.VersionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.lock: %v\n", err)
		osExit(1)
	}

	// Use global storage for shell-pulled repos
	store := localindex.NewStore()
	cacheDir := store.GlobalRepoPath(env.ID, tag)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create cache directory: %v\n", err)
		osExit(1)
	}

	// Write files
	pixiTomlBytes := []byte(pixiToml)
	pixiLockBytes := []byte(pixiLock)
	if err := os.WriteFile(filepath.Join(cacheDir, "pixi.toml"), pixiTomlBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.toml: %v\n", err)
		osExit(1)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "pixi.lock"), pixiLockBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.lock: %v\n", err)
		osExit(1)
	}

	// Write .nebi metadata
	tomlDigest := nebifile.ComputeDigest(pixiTomlBytes)
	lockDigest := nebifile.ComputeDigest(pixiLockBytes)
	nf := nebifile.NewFromPull(
		repoName, tag, "", "",
		fmt.Sprintf("%d", latestVersion.VersionNumber), "",
	)
	nebifile.Write(cacheDir, nf)

	// Add to local index
	store.AddEntry(localindex.Entry{
		SpecName:    repoName,
		VersionName: tag,
		VersionID:   fmt.Sprintf("%d", latestVersion.VersionNumber),
		Path:        cacheDir,
		PulledAt:    time.Now(),
		Layers: map[string]string{
			"pixi.toml": tomlDigest,
			"pixi.lock": lockDigest,
		},
	})

	fmt.Printf("Cached at %s\n", cacheDir)
	return cacheDir
}

// checkShellDrift checks for local modifications and warns the user.
func checkShellDrift(dir string) {
	ws, err := drift.Check(dir)
	if err != nil {
		return // No .nebi file or other issue - silently proceed
	}

	if ws.Overall == drift.StatusModified {
		fmt.Fprintln(os.Stderr, "Warning: Local repo has been modified since pull")
		for _, f := range ws.Files {
			if f.Status == drift.StatusModified {
				fmt.Fprintf(os.Stderr, "  modified: %s\n", f.Filename)
			}
		}
		fmt.Fprintln(os.Stderr, "")
	}
}

// getRepoCacheDir returns the cache directory for a repo.
func getRepoCacheDir(repoName string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	repoDir := filepath.Join(cacheDir, "nebi", "repos", repoName)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return "", err
	}

	return repoDir, nil
}
