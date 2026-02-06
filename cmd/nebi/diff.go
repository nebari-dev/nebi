package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/diff"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var diffLock bool

var diffCmd = &cobra.Command{
	Use:   "diff <ref-a> [ref-b] [--lock]",
	Short: "Compare workspace specifications between two sources",
	Long: `Compare pixi.toml (and pixi.lock with --lock) between two references.
Each reference can be:
  - A path (contains a slash): ./dir, /tmp/project, foo/bar
  - A global workspace name (bare word): data-science
  - A server ref (contains a colon): myworkspace:v1

If no refs are given, compares the current directory against the last
pushed/pulled origin.

If only one ref is given, it is compared against the current directory.

Examples:
  nebi diff                                    # local vs origin
  nebi diff ./other-project                    # other dir vs cwd
  nebi diff ./project-a ./project-b            # two local dirs
  nebi diff data-science                       # global workspace vs cwd
  nebi diff myworkspace:v1                     # server version vs cwd
  nebi diff myworkspace:v1 myworkspace:v2      # two server versions
  nebi diff myworkspace:v1 ./local-dir         # server vs local dir

Use --lock to also compare pixi.lock files.`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffLock, "lock", false, "Also diff pixi.lock files")
}

// diffSource represents a resolved source of pixi files for diffing.
type diffSource struct {
	label string // label prefix for diff output (e.g. "a", "b", "server")
	toml  string // pixi.toml content
	lock  string // pixi.lock content (may be empty)
}

func runDiff(cmd *cobra.Command, args []string) error {
	var refA, refB string

	switch len(args) {
	case 0:
		// No args — diff origin vs local (origin is baseline, local shows changes)
		origin, err := lookupOrigin()
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no origin set; use 'nebi diff <ref>' or push/pull first")
		}
		refA = origin.OriginName + ":" + origin.OriginTag
		refB = "."
	case 1:
		refA = "."
		refB = args[0]
	default:
		refA = args[0]
		refB = args[1]
	}

	srcA, err := resolveSource(refA, "")
	if err != nil {
		return fmt.Errorf("resolving %s: %w", refA, err)
	}

	srcB, err := resolveSource(refB, "")
	if err != nil {
		return fmt.Errorf("resolving %s: %w", refB, err)
	}

	hasOutput := false

	// Semantic TOML diff
	tomlDiff, err := diff.CompareToml([]byte(srcA.toml), []byte(srcB.toml))
	if err != nil {
		return fmt.Errorf("comparing pixi.toml: %w", err)
	}

	if tomlDiff.HasChanges() {
		fmt.Print(diff.FormatUnifiedDiff(tomlDiff, srcA.label, srcB.label))
		hasOutput = true
	}

	// Lock file diff
	if srcA.lock != srcB.lock {
		lockSummary, _ := diff.CompareLock([]byte(srcA.lock), []byte(srcB.lock))
		if diffLock && lockSummary != nil {
			fmt.Println()
			fmt.Print(diff.FormatLockDiffText(lockSummary))
			hasOutput = true
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
			hasOutput = true
		}
	}

	if !hasOutput {
		fmt.Fprintln(os.Stderr, "No differences.")
	}
	return nil
}

// resolveSource resolves a ref (directory, workspace name, or workspace:tag) into a diffSource.
func resolveSource(ref, defaultLabel string) (*diffSource, error) {
	// 1. Local directory path (must contain a slash, e.g. ./foo, /tmp/foo, foo/bar)
	if isPath(ref) {
		return resolveLocalSource(ref, defaultLabel)
	}

	// 2. Global workspace name (check store before assuming server ref)
	if !strings.Contains(ref, ":") {
		s, err := store.New()
		if err == nil {
			defer s.Close()
			ws, err := s.FindGlobalWorkspaceByName(ref)
			if err == nil && ws != nil {
				return resolveLocalSource(ws.Path, ref)
			}
		}
	}

	// 3. Server ref (workspace:tag)
	return resolveServerSource(ref)
}

func resolveLocalSource(dir, defaultLabel string) (*diffSource, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	label := defaultLabel
	if dir == "." && label == "" {
		label = "local"
	} else if label == "" {
		label = dir
	}

	toml, err := os.ReadFile(filepath.Join(absDir, "pixi.toml"))
	if err != nil {
		return nil, fmt.Errorf("reading pixi.toml from %s: %w", absDir, err)
	}

	var lock string
	lockData, err := os.ReadFile(filepath.Join(absDir, "pixi.lock"))
	if err == nil {
		lock = string(lockData)
	}

	return &diffSource{
		label: label,
		toml:  string(toml),
		lock:  lock,
	}, nil
}

func resolveServerSource(ref string) (*diffSource, error) {
	wsName, tag := parseWsRef(ref)

	client, err := getAuthenticatedClient()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		return nil, err
	}

	versionNumber, err := resolveVersionNumber(client, ctx, ws.ID, wsName, tag)
	if err != nil {
		return nil, err
	}

	toml, err := client.GetVersionPixiToml(ctx, ws.ID, versionNumber)
	if err != nil {
		return nil, fmt.Errorf("fetching pixi.toml: %w", err)
	}

	lock, _ := client.GetVersionPixiLock(ctx, ws.ID, versionNumber)

	label := wsName
	if tag != "" {
		label = wsName + ":" + tag
	}

	return &diffSource{
		label: label,
		toml:  toml,
		lock:  lock,
	}, nil
}

// resolveVersionNumber resolves a tag or latest version to a version number.
func resolveVersionNumber(client *cliclient.Client, ctx context.Context, wsID, wsName, tag string) (int32, error) {
	if tag != "" {
		tags, err := client.GetWorkspaceTags(ctx, wsID)
		if err != nil {
			return 0, fmt.Errorf("getting tags: %w", err)
		}
		for _, t := range tags {
			if t.Tag == tag {
				return int32(t.VersionNumber), nil
			}
		}
		return 0, fmt.Errorf("tag %q not found for workspace %q", tag, wsName)
	}

	versions, err := client.GetWorkspaceVersions(ctx, wsID)
	if err != nil {
		return 0, fmt.Errorf("getting versions: %w", err)
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("workspace %q has no versions", wsName)
	}
	latest := versions[0]
	for _, v := range versions {
		if v.VersionNumber > latest.VersionNumber {
			latest = v
		}
	}
	return latest.VersionNumber, nil
}

// isPath returns true if ref looks like a filesystem path.
func isPath(ref string) bool {
	return ref == "." || ref == ".." || strings.Contains(ref, "/") || strings.Contains(ref, string(filepath.Separator))
}
