package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var shellPixiEnv string

var shellCmd = &cobra.Command{
	Use:   "shell [<workspace>[:<tag>]]",
	Short: "Activate workspace shell",
	Long: `Activate a workspace shell using pixi shell.

When run without arguments in a directory with .nebi metadata, uses the
local workspace. When given a workspace reference, looks up the local
index (preferring global copies) and falls back to pulling from server.

Drift detection warns if local files have been modified since pull.

Examples:
  # Shell from current directory (reads .nebi metadata)
  nebi shell

  # Shell into specific workspace by name
  nebi shell myworkspace:v1.0.0

  # Shell into specific pixi environment
  nebi shell myworkspace:v1.0.0 -e dev`,
	Args: cobra.MaximumNArgs(1),
	Run:  runShell,
}

func init() {
	shellCmd.Flags().StringVarP(&shellPixiEnv, "env", "e", "", "Pixi environment name")
}

func runShell(cmd *cobra.Command, args []string) {
	var shellDir string

	if len(args) == 0 {
		// No argument - use current directory
		shellDir = resolveShellFromCwd()
	} else {
		// Parse workspace:tag format
		workspaceName, tag, err := parseWorkspaceRef(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		shellDir = resolveShellFromRef(workspaceName, tag)
	}

	// Check for drift and warn
	checkShellDrift(shellDir)

	// Run pixi shell
	execPixiShell(shellDir, shellPixiEnv)
}

// resolveShellFromCwd resolves a shell directory from the current working directory.
func resolveShellFromCwd() string {
	absDir, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check for .nebi metadata
	if nebifile.Exists(absDir) {
		return absDir
	}

	// Check for pixi.toml (can still shell without .nebi)
	if _, err := os.Stat(filepath.Join(absDir, "pixi.toml")); err == nil {
		return absDir
	}

	fmt.Fprintf(os.Stderr, "Error: No workspace found in current directory\n")
	fmt.Fprintln(os.Stderr, "Run 'nebi pull <workspace>:<tag>' to pull a workspace, or specify one: 'nebi shell <workspace>:<tag>'")
	os.Exit(1)
	return ""
}

// resolveShellFromRef resolves a shell directory from a workspace reference.
// Priority: global copy > most recent local copy > pull from server.
func resolveShellFromRef(workspaceName, tag string) string {
	store := localindex.NewStore()

	// First, check for a global copy
	if tag != "" {
		global, err := store.FindGlobal(workspaceName, tag)
		if err == nil && global != nil {
			if _, err := os.Stat(global.Path); err == nil {
				refStr := workspaceName + ":" + tag
				fmt.Printf("Using global copy of %s\n", refStr)
				return global.Path
			}
		}
	}

	// Check local index for any matching entries
	if tag != "" {
		matches, err := store.FindByWorkspaceTag(workspaceName, tag)
		if err == nil && len(matches) > 0 {
			// Filter to entries that still exist
			var valid []localindex.WorkspaceEntry
			for _, m := range matches {
				if _, err := os.Stat(m.Path); err == nil {
					valid = append(valid, m)
				}
			}

			if len(valid) == 1 {
				fmt.Printf("Using local copy at %s\n", valid[0].Path)
				return valid[0].Path
			}
			if len(valid) > 1 {
				// Use most recent
				best := valid[0]
				for _, v := range valid[1:] {
					if v.PulledAt.After(best.PulledAt) {
						best = v
					}
				}
				fmt.Printf("Using most recent local copy at %s (pulled %s)\n",
					best.Path, formatTimeAgo(best.PulledAt))
				return best.Path
			}
		}
	}

	// Not in local index - pull from server
	return pullForShell(workspaceName, tag)
}

// pullForShell pulls a workspace from the server for shell activation.
func pullForShell(workspaceName, tag string) string {
	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace by name
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get versions to find the right one
	versions, err := client.GetEnvironmentVersions(ctx, env.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
		os.Exit(1)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Workspace %q has no versions\n", workspaceName)
		os.Exit(1)
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

	refStr := workspaceName + ":" + tag
	fmt.Printf("Pulling %s (version %d)...\n", refStr, latestVersion.VersionNumber)

	// Get content
	pixiToml, err := client.GetVersionPixiToml(ctx, env.ID, latestVersion.VersionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.toml: %v\n", err)
		os.Exit(1)
	}
	pixiLock, err := client.GetVersionPixiLock(ctx, env.ID, latestVersion.VersionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.lock: %v\n", err)
		os.Exit(1)
	}

	// Use global storage for shell-pulled workspaces
	store := localindex.NewStore()
	cacheDir := store.GlobalWorkspacePath(env.ID, tag)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create cache directory: %v\n", err)
		os.Exit(1)
	}

	// Write files
	pixiTomlBytes := []byte(pixiToml)
	pixiLockBytes := []byte(pixiLock)
	if err := os.WriteFile(filepath.Join(cacheDir, "pixi.toml"), pixiTomlBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.toml: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "pixi.lock"), pixiLockBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.lock: %v\n", err)
		os.Exit(1)
	}

	// Write .nebi metadata
	tomlDigest := nebifile.ComputeDigest(pixiTomlBytes)
	lockDigest := nebifile.ComputeDigest(pixiLockBytes)
	nf := nebifile.NewFromPull(
		workspaceName, tag, "", "",
		latestVersion.VersionNumber, "",
		tomlDigest, int64(len(pixiTomlBytes)),
		lockDigest, int64(len(pixiLockBytes)),
	)
	nebifile.Write(cacheDir, nf)

	// Add to local index
	store.AddEntry(localindex.WorkspaceEntry{
		Workspace:       workspaceName,
		Tag:             tag,
		Path:            cacheDir,
		IsGlobal:        true,
		ServerVersionID: latestVersion.VersionNumber,
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
		fmt.Fprintln(os.Stderr, "Warning: Local workspace has been modified since pull")
		for _, f := range ws.Files {
			if f.Status == drift.StatusModified {
				fmt.Fprintf(os.Stderr, "  modified: %s\n", f.Filename)
			}
		}
		fmt.Fprintln(os.Stderr, "")
	}
}

// getWorkspaceCacheDir returns the cache directory for a workspace.
func getWorkspaceCacheDir(workspaceName string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	workspaceDir := filepath.Join(cacheDir, "nebi", "workspaces", workspaceName)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", err
	}

	return workspaceDir, nil
}
