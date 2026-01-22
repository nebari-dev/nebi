package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/diff"
	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var (
	diffRemote  bool
	diffJSON    bool
	diffLock    bool
	diffToml    bool
	diffPath    string
)

var diffCmd = &cobra.Command{
	Use:   "diff [source] [target]",
	Short: "Show workspace differences",
	Long: `Show detailed differences between workspace versions.

While 'nebi status' answers "has anything changed?", 'nebi diff' answers
"what exactly changed?".

Usage patterns:
  nebi diff                    Local changes vs what was pulled
  nebi diff --remote           Local vs current remote tag

Examples:
  # Show local changes vs origin
  nebi diff

  # Show changes vs current remote
  nebi diff --remote

  # JSON output for scripting
  nebi diff --json

  # Only show pixi.toml changes
  nebi diff --toml

  # Check workspace at a specific path
  nebi diff -C /path/to/workspace`,
	Args: cobra.MaximumNArgs(2),
	Run:  runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffRemote, "remote", false, "Compare against current remote tag")
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output as JSON")
	diffCmd.Flags().BoolVar(&diffLock, "lock", false, "Show full lock file diff")
	diffCmd.Flags().BoolVar(&diffToml, "toml", false, "Show only pixi.toml diff")
	diffCmd.Flags().StringVarP(&diffPath, "path", "C", ".", "Workspace directory path")
}

func runDiff(cmd *cobra.Command, args []string) {
	dir := diffPath

	// Resolve absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Read .nebi metadata
	nf, err := nebifile.Read(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Hint: Run 'nebi pull' first to create a workspace with tracking metadata.")
		os.Exit(2)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find workspace on server
	env, err := findWorkspaceByName(client, ctx, nf.Origin.Workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	// Determine what to fetch for comparison
	var versionContent *drift.VersionContent
	var sourceLabel string

	if diffRemote {
		// Fetch current tag content
		versionContent, err = drift.FetchByTag(ctx, client, nf.Origin.Workspace, nf.Origin.Tag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to fetch remote content: %v\n", err)
			os.Exit(2)
		}
		sourceLabel = fmt.Sprintf("remote (%s:%s, current)", nf.Origin.Workspace, nf.Origin.Tag)
	} else {
		// Fetch origin content (by version ID - immutable)
		versionContent, err = drift.FetchVersionContent(ctx, client, env.ID, nf.Origin.ServerVersionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to fetch origin content: %v\n", err)
			fmt.Fprintln(os.Stderr, "Hint: The origin version may no longer be available on the server.")
			os.Exit(2)
		}
		sourceLabel = fmt.Sprintf("pulled (%s:%s, %s)", nf.Origin.Workspace, nf.Origin.Tag, truncateDigest(nf.Origin.ManifestDigest))
	}

	// Read local files
	localPixiToml, err := os.ReadFile(filepath.Join(absDir, "pixi.toml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to read local pixi.toml: %v\n", err)
		os.Exit(2)
	}

	localPixiLock, _ := os.ReadFile(filepath.Join(absDir, "pixi.lock"))

	// Compute TOML diff
	tomlDiff, err := diff.CompareToml([]byte(versionContent.PixiToml), localPixiToml)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compare pixi.toml: %v\n", err)
		os.Exit(2)
	}

	// Output
	targetLabel := "local"

	if diffJSON {
		outputDiffJSON(nf, tomlDiff, versionContent, localPixiLock, sourceLabel, absDir)
	} else {
		outputDiffText(tomlDiff, versionContent, localPixiLock, sourceLabel, targetLabel)
	}

	// Exit code
	if tomlDiff.HasChanges() || !bytesEqual([]byte(versionContent.PixiLock), localPixiLock) {
		os.Exit(1)
	}
}

func outputDiffText(tomlDiff *diff.TomlDiff, versionContent *drift.VersionContent, localLock []byte, sourceLabel, targetLabel string) {
	if !diffToml {
		// Show TOML diff
		unified := diff.FormatUnifiedDiff(tomlDiff, sourceLabel, targetLabel)
		if unified != "" {
			fmt.Print(unified)
		}
	} else {
		// Show only TOML diff
		unified := diff.FormatUnifiedDiff(tomlDiff, sourceLabel, targetLabel)
		if unified != "" {
			fmt.Print(unified)
		} else {
			fmt.Println("No changes in pixi.toml")
		}
		return
	}

	// Lock file summary
	if !diffLock {
		// Just show if lock file changed
		if !bytesEqual([]byte(versionContent.PixiLock), localLock) {
			fmt.Println()
			fmt.Println("@@ pixi.lock (changed) @@")
			fmt.Println("[Use --lock for full lock file details]")
		}
	}

	if !tomlDiff.HasChanges() && bytesEqual([]byte(versionContent.PixiLock), localLock) {
		fmt.Println("No changes detected")
	}
}

func outputDiffJSON(nf *nebifile.NebiFile, tomlDiff *diff.TomlDiff, versionContent *drift.VersionContent, localLock []byte, sourceLabel, absDir string) {
	source := diff.DiffRefJSON{
		Type:      "pulled",
		Workspace: nf.Origin.Workspace,
		Tag:       nf.Origin.Tag,
		Digest:    nf.Origin.ManifestDigest,
	}
	if diffRemote {
		source.Type = "remote"
	}

	target := diff.DiffRefJSON{
		Type: "local",
		Path: absDir,
	}

	var lockSummary *diff.LockSummary
	if !bytesEqual([]byte(versionContent.PixiLock), localLock) {
		lockSummary = &diff.LockSummary{
			PackagesAdded:   0,
			PackagesRemoved: 0,
			PackagesUpdated: 0,
		}
	}

	data, err := diff.FormatDiffJSON(source, target, tomlDiff, lockSummary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to marshal JSON: %v\n", err)
		os.Exit(2)
	}
	fmt.Println(string(data))
}

func truncateDigest(digest string) string {
	if len(digest) > 19 {
		return digest[:19] + "..."
	}
	return digest
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
