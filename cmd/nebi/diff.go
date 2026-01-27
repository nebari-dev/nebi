package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aktech/darb/internal/diff"
	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var (
	diffRemote bool
	diffJSON   bool
	diffLock   bool
	diffToml   bool
	diffPath   string
)

var diffCmd = &cobra.Command{
	Use:   "diff [env:version] [env:version]",
	Short: "Show environment differences",
	Long: `Show detailed differences between environment versions.

While 'nebi status' answers "has anything changed?", 'nebi diff' answers
"what exactly changed?".

Arguments can be env:version references (fetched from server) or local
paths (read directly from disk). Paths are detected by prefix: /, ./, ../,
~, or the literal ".".

Usage patterns:
  nebi diff                              Local changes vs what was pulled
  nebi diff --remote                     Local vs current remote version
  nebi diff env:v1.0 env:v2.0           Compare two remote references
  nebi diff env:v1.0                    Compare remote ref vs local
  nebi diff ./project-a ./project-b      Compare two local paths
  nebi diff . ~/other-project            Current dir vs another local path
  nebi diff ~/local data-science:v2.0    Local path vs remote reference

Examples:
  # Show local changes vs origin
  nebi diff

  # Show changes vs current remote
  nebi diff --remote

  # Compare two versions of an environment
  nebi diff data-science:v1.0 data-science:v2.0

  # Compare remote ref vs local
  nebi diff data-science:v1.0

  # Compare two local environment directories
  nebi diff ./experiment-1 ./experiment-2

  # Include lock file package-level diff
  nebi diff --lock

  # JSON output for scripting
  nebi diff --json

  # Check environment at a specific path
  nebi diff -C /path/to/env`,
	Args: cobra.MaximumNArgs(2),
	Run:  runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffRemote, "remote", false, "Compare against current remote version")
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output as JSON")
	diffCmd.Flags().BoolVar(&diffLock, "lock", false, "Show lock file package-level diff")
	diffCmd.Flags().BoolVar(&diffToml, "toml", false, "Show only pixi.toml diff")
	diffCmd.Flags().StringVarP(&diffPath, "path", "C", ".", "Environment directory path")
}

func runDiff(cmd *cobra.Command, args []string) {
	switch len(args) {
	case 2:
		srcPath := isPathLike(args[0])
		tgtPath := isPathLike(args[1])
		switch {
		case srcPath && tgtPath:
			runDiffTwoPaths(args[0], args[1])
		case srcPath && !tgtPath:
			runDiffPathVsRef(args[0], args[1])
		case !srcPath && tgtPath:
			runDiffRefVsPath(args[0], args[1])
		default:
			runDiffTwoRefs(args[0], args[1])
		}
	case 1:
		if isPathLike(args[0]) {
			// nebi diff ./path — compare path vs origin (treat as source)
			runDiffTwoPaths(diffPath, args[0])
		} else {
			// nebi diff ref1 — compare remote ref vs local
			runDiffRefVsLocal(args[0])
		}
	default:
		// nebi diff — local vs origin (or --remote)
		runDiffLocal()
	}
}

// runDiffLocal handles: nebi diff (no args) and nebi diff --remote
func runDiffLocal() {
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
		fmt.Fprintln(os.Stderr, "Hint: Run 'nebi pull' first to create an environment with tracking metadata.")
		osExit(2)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find environment on server
	env, err := findEnvByName(client, ctx, nf.Origin.Repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}

	// Determine what to fetch for comparison
	var versionContent *drift.VersionContent
	var sourceLabel string

	if diffRemote {
		// Fetch current version content
		versionContent, err = drift.FetchByTag(ctx, client, nf.Origin.Repo, nf.Origin.Tag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to fetch remote content: %v\n", err)
			osExit(2)
		}
		sourceLabel = fmt.Sprintf("remote (%s:%s, current)", nf.Origin.Repo, nf.Origin.Tag)
	} else {
		// Fetch origin content (by version ID - immutable)
		versionContent, err = drift.FetchVersionContent(ctx, client, env.ID, nf.Origin.ServerVersionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to fetch origin content: %v\n", err)
			fmt.Fprintln(os.Stderr, "Hint: The origin version may no longer be available on the server.")
			osExit(2)
		}
		sourceLabel = fmt.Sprintf("pulled (%s:%s, %s)", nf.Origin.Repo, nf.Origin.Tag, truncateDigest(nf.Origin.ManifestDigest))
	}

	// Read local files
	localPixiToml, err := os.ReadFile(filepath.Join(absDir, "pixi.toml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to read local pixi.toml: %v\n", err)
		osExit(2)
	}

	localPixiLock, _ := os.ReadFile(filepath.Join(absDir, "pixi.lock"))

	// Compute diffs
	tomlDiff, err := diff.CompareToml([]byte(versionContent.PixiToml), localPixiToml)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compare pixi.toml: %v\n", err)
		osExit(2)
	}

	// Compute lock diff
	var lockSummary *diff.LockSummary
	if !bytesEqual([]byte(versionContent.PixiLock), localPixiLock) {
		lockSummary, _ = diff.CompareLock([]byte(versionContent.PixiLock), localPixiLock)
	}

	// Output
	targetLabel := "local"

	if diffJSON {
		outputDiffJSON(nf, tomlDiff, lockSummary, sourceLabel, absDir)
	} else {
		outputDiffText(tomlDiff, lockSummary, []byte(versionContent.PixiLock), localPixiLock, sourceLabel, targetLabel)
	}

	// Exit code
	if tomlDiff.HasChanges() || !bytesEqual([]byte(versionContent.PixiLock), localPixiLock) {
		osExit(1)
	}
}

// runDiffRefVsLocal compares a remote reference against local environment.
func runDiffRefVsLocal(ref string) {
	envName, version, err := parseEnvRef(ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}
	if version == "" {
		fmt.Fprintf(os.Stderr, "Error: version is required in reference (e.g., %s:v1.0)\n", envName)
		osExit(2)
	}

	dir := diffPath
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Fetch the remote reference content
	vc, err := drift.FetchByTag(ctx, client, envName, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to fetch %s:%s: %v\n", envName, version, err)
		osExit(2)
	}

	// Read local files
	localPixiToml, err := os.ReadFile(filepath.Join(absDir, "pixi.toml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to read local pixi.toml: %v\n", err)
		osExit(2)
	}
	localPixiLock, _ := os.ReadFile(filepath.Join(absDir, "pixi.lock"))

	// Compute diffs
	tomlDiff, err := diff.CompareToml([]byte(vc.PixiToml), localPixiToml)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compare pixi.toml: %v\n", err)
		osExit(2)
	}

	var lockSummary *diff.LockSummary
	if !bytesEqual([]byte(vc.PixiLock), localPixiLock) {
		lockSummary, _ = diff.CompareLock([]byte(vc.PixiLock), localPixiLock)
	}

	sourceLabel := fmt.Sprintf("%s:%s", envName, version)
	targetLabel := "local"

	if diffJSON {
		outputDiffJSONRefs(
			diff.DiffRefJSON{Type: "version", Repo: envName, Tag: version},
			diff.DiffRefJSON{Type: "local", Path: absDir},
			tomlDiff, lockSummary,
		)
	} else {
		outputDiffText(tomlDiff, lockSummary, []byte(vc.PixiLock), localPixiLock, sourceLabel, targetLabel)
	}

	if tomlDiff.HasChanges() || !bytesEqual([]byte(vc.PixiLock), localPixiLock) {
		osExit(1)
	}
}

// runDiffTwoRefs compares two remote environment references.
func runDiffTwoRefs(ref1, ref2 string) {
	env1, version1, err := parseEnvRef(ref1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}
	if version1 == "" {
		fmt.Fprintf(os.Stderr, "Error: version is required in first reference (e.g., %s:v1.0)\n", env1)
		osExit(2)
	}

	env2, version2, err := parseEnvRef(ref2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}
	if version2 == "" {
		fmt.Fprintf(os.Stderr, "Error: version is required in second reference (e.g., %s:v2.0)\n", env2)
		osExit(2)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Fetch both references
	vc1, err := drift.FetchByTag(ctx, client, env1, version1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to fetch %s:%s: %v\n", env1, version1, err)
		osExit(2)
	}

	vc2, err := drift.FetchByTag(ctx, client, env2, version2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to fetch %s:%s: %v\n", env2, version2, err)
		osExit(2)
	}

	// Compute diffs
	tomlDiff, err := diff.CompareToml([]byte(vc1.PixiToml), []byte(vc2.PixiToml))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compare pixi.toml: %v\n", err)
		osExit(2)
	}

	var lockSummary *diff.LockSummary
	if !bytesEqual([]byte(vc1.PixiLock), []byte(vc2.PixiLock)) {
		lockSummary, _ = diff.CompareLock([]byte(vc1.PixiLock), []byte(vc2.PixiLock))
	}

	sourceLabel := fmt.Sprintf("%s:%s", env1, version1)
	targetLabel := fmt.Sprintf("%s:%s", env2, version2)

	if diffJSON {
		outputDiffJSONRefs(
			diff.DiffRefJSON{Type: "version", Repo: env1, Tag: version1},
			diff.DiffRefJSON{Type: "version", Repo: env2, Tag: version2},
			tomlDiff, lockSummary,
		)
	} else {
		outputDiffText(tomlDiff, lockSummary, []byte(vc1.PixiLock), []byte(vc2.PixiLock), sourceLabel, targetLabel)
	}

	if tomlDiff.HasChanges() || !bytesEqual([]byte(vc1.PixiLock), []byte(vc2.PixiLock)) {
		osExit(1)
	}
}

func outputDiffText(tomlDiff *diff.TomlDiff, lockSummary *diff.LockSummary, sourceLock, targetLock []byte, sourceLabel, targetLabel string) {
	if diffToml {
		// Show only TOML diff
		unified := diff.FormatUnifiedDiff(tomlDiff, sourceLabel, targetLabel)
		if unified != "" {
			fmt.Print(unified)
		} else {
			fmt.Println("No changes in pixi.toml")
		}
		return
	}

	// Show TOML diff
	unified := diff.FormatUnifiedDiff(tomlDiff, sourceLabel, targetLabel)
	if unified != "" {
		fmt.Print(unified)
	}

	// Lock file handling
	if !bytesEqual(sourceLock, targetLock) {
		if diffLock && lockSummary != nil {
			// Show package-level lock diff
			fmt.Println()
			fmt.Print(diff.FormatLockDiffText(lockSummary))
		} else {
			fmt.Println()
			fmt.Println("@@ pixi.lock (changed) @@")
			if lockSummary != nil {
				total := lockSummary.PackagesAdded + lockSummary.PackagesRemoved + lockSummary.PackagesUpdated
				if total > 0 {
					fmt.Printf("  %d packages changed", total)
					if lockSummary.PackagesAdded > 0 {
						fmt.Printf(", %d added", lockSummary.PackagesAdded)
					}
					if lockSummary.PackagesRemoved > 0 {
						fmt.Printf(", %d removed", lockSummary.PackagesRemoved)
					}
					if lockSummary.PackagesUpdated > 0 {
						fmt.Printf(", %d updated", lockSummary.PackagesUpdated)
					}
					fmt.Println()
				}
			}
			fmt.Println("[Use --lock for full lock file details]")
		}
	}

	if !tomlDiff.HasChanges() && bytesEqual(sourceLock, targetLock) {
		fmt.Println("No changes detected")
	}
}

func outputDiffJSON(nf *nebifile.NebiFile, tomlDiff *diff.TomlDiff, lockSummary *diff.LockSummary, sourceLabel, absDir string) {
	source := diff.DiffRefJSON{
		Type:   "pulled",
		Repo:   nf.Origin.Repo,
		Tag:    nf.Origin.Tag,
		Digest: nf.Origin.ManifestDigest,
	}
	if diffRemote {
		source.Type = "remote"
	}

	target := diff.DiffRefJSON{
		Type: "local",
		Path: absDir,
	}

	data, err := diff.FormatDiffJSON(source, target, tomlDiff, lockSummary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to marshal JSON: %v\n", err)
		osExit(2)
	}
	fmt.Println(string(data))
}

func outputDiffJSONRefs(source, target diff.DiffRefJSON, tomlDiff *diff.TomlDiff, lockSummary *diff.LockSummary) {
	data, err := diff.FormatDiffJSON(source, target, tomlDiff, lockSummary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to marshal JSON: %v\n", err)
		osExit(2)
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

// isPathLike returns true if the argument looks like a filesystem path
// rather than an env:version reference.
// Path-like: starts with /, ./, ../, ~, or is exactly "."
func isPathLike(arg string) bool {
	if arg == "." {
		return true
	}
	return strings.HasPrefix(arg, "/") ||
		strings.HasPrefix(arg, "./") ||
		strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, "~")
}

// resolvePath resolves a path argument to an absolute path.
// Handles ~ expansion and relative paths.
func resolvePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// readLocalEnv reads pixi.toml and pixi.lock from a directory.
// Returns an error if pixi.toml doesn't exist. pixi.lock is optional.
func readLocalEnv(dir string) (pixiToml, pixiLock []byte, err error) {
	pixiToml, err = os.ReadFile(filepath.Join(dir, "pixi.toml"))
	if err != nil {
		return nil, nil, fmt.Errorf("%s/pixi.toml: %w", dir, err)
	}
	pixiLock, _ = os.ReadFile(filepath.Join(dir, "pixi.lock"))
	return pixiToml, pixiLock, nil
}

// runDiffTwoPaths compares two local environment directories.
func runDiffTwoPaths(path1, path2 string) {
	abs1, err := resolvePath(path1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}
	abs2, err := resolvePath(path2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}

	toml1, lock1, err := readLocalEnv(abs1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}
	toml2, lock2, err := readLocalEnv(abs2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}

	tomlDiff, err := diff.CompareToml(toml1, toml2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compare pixi.toml: %v\n", err)
		osExit(2)
	}

	var lockSummary *diff.LockSummary
	if !bytesEqual(lock1, lock2) {
		lockSummary, _ = diff.CompareLock(lock1, lock2)
	}

	sourceLabel := abs1
	targetLabel := abs2

	if diffJSON {
		outputDiffJSONRefs(
			diff.DiffRefJSON{Type: "local", Path: abs1},
			diff.DiffRefJSON{Type: "local", Path: abs2},
			tomlDiff, lockSummary,
		)
	} else {
		outputDiffText(tomlDiff, lockSummary, lock1, lock2, sourceLabel, targetLabel)
	}

	if tomlDiff.HasChanges() || !bytesEqual(lock1, lock2) {
		osExit(1)
	}
}

// runDiffPathVsRef compares a local path (source) against a remote reference (target).
func runDiffPathVsRef(path, ref string) {
	abs, err := resolvePath(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}

	envName, version, err := parseEnvRef(ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}
	if version == "" {
		fmt.Fprintf(os.Stderr, "Error: version is required in reference (e.g., %s:v1.0)\n", envName)
		osExit(2)
	}

	localToml, localLock, err := readLocalEnv(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	vc, err := drift.FetchByTag(ctx, client, envName, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to fetch %s:%s: %v\n", envName, version, err)
		osExit(2)
	}

	tomlDiff, err := diff.CompareToml(localToml, []byte(vc.PixiToml))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compare pixi.toml: %v\n", err)
		osExit(2)
	}

	var lockSummary *diff.LockSummary
	if !bytesEqual(localLock, []byte(vc.PixiLock)) {
		lockSummary, _ = diff.CompareLock(localLock, []byte(vc.PixiLock))
	}

	sourceLabel := abs
	targetLabel := fmt.Sprintf("%s:%s", envName, version)

	if diffJSON {
		outputDiffJSONRefs(
			diff.DiffRefJSON{Type: "local", Path: abs},
			diff.DiffRefJSON{Type: "version", Repo: envName, Tag: version},
			tomlDiff, lockSummary,
		)
	} else {
		outputDiffText(tomlDiff, lockSummary, localLock, []byte(vc.PixiLock), sourceLabel, targetLabel)
	}

	if tomlDiff.HasChanges() || !bytesEqual(localLock, []byte(vc.PixiLock)) {
		osExit(1)
	}
}

// runDiffRefVsPath compares a remote reference (source) against a local path (target).
func runDiffRefVsPath(ref, path string) {
	abs, err := resolvePath(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}

	envName, version, err := parseEnvRef(ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}
	if version == "" {
		fmt.Fprintf(os.Stderr, "Error: version is required in reference (e.g., %s:v1.0)\n", envName)
		osExit(2)
	}

	localToml, localLock, err := readLocalEnv(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(2)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	vc, err := drift.FetchByTag(ctx, client, envName, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to fetch %s:%s: %v\n", envName, version, err)
		osExit(2)
	}

	tomlDiff, err := diff.CompareToml([]byte(vc.PixiToml), localToml)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compare pixi.toml: %v\n", err)
		osExit(2)
	}

	var lockSummary *diff.LockSummary
	if !bytesEqual([]byte(vc.PixiLock), localLock) {
		lockSummary, _ = diff.CompareLock([]byte(vc.PixiLock), localLock)
	}

	sourceLabel := fmt.Sprintf("%s:%s", envName, version)
	targetLabel := abs

	if diffJSON {
		outputDiffJSONRefs(
			diff.DiffRefJSON{Type: "version", Repo: envName, Tag: version},
			diff.DiffRefJSON{Type: "local", Path: abs},
			tomlDiff, lockSummary,
		)
	} else {
		outputDiffText(tomlDiff, lockSummary, []byte(vc.PixiLock), localLock, sourceLabel, targetLabel)
	}

	if tomlDiff.HasChanges() || !bytesEqual([]byte(vc.PixiLock), localLock) {
		osExit(1)
	}
}
