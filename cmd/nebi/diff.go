package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
)

var diffLock bool
var diffServer string

var diffCmd = &cobra.Command{
	Use:   "diff <ref-a> [ref-b] [--lock]",
	Short: "Compare pixi files between two sources",
	Long: `Compare pixi.toml (and pixi.lock with --lock) between two references.
Each reference can be a local directory or a server workspace.

If only one ref is given, it is compared against the current directory.

Examples:
  nebi diff ./other-project                    # other dir vs cwd
  nebi diff ./project-a ./project-b            # two local dirs
  nebi diff myworkspace:v1 -s work             # server version vs cwd
  nebi diff myworkspace:v1 myworkspace:v2 -s work  # two server versions
  nebi diff myworkspace:v1 ./local-dir -s work # server vs local dir

Use --lock to also compare pixi.lock files.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDiff,
}

func init() {
	diffCmd.Flags().StringVarP(&diffServer, "server", "s", "", "Server name or URL (uses default if not set)")
	diffCmd.Flags().BoolVar(&diffLock, "lock", false, "Also diff pixi.lock files")
}

// diffSource represents a resolved source of pixi files for diffing.
type diffSource struct {
	label string            // label prefix for diff output (e.g. "a", "b", "server")
	toml  string            // pixi.toml content
	lock  string            // pixi.lock content (may be empty)
}

func runDiff(cmd *cobra.Command, args []string) error {
	refA := args[0]
	refB := "."
	if len(args) == 2 {
		refB = args[1]
	}

	srcA, err := resolveSource(refA, "a")
	if err != nil {
		return fmt.Errorf("resolving %s: %w", refA, err)
	}

	srcB, err := resolveSource(refB, "b")
	if err != nil {
		return fmt.Errorf("resolving %s: %w", refB, err)
	}

	files := []string{"pixi.toml"}
	if diffLock {
		files = append(files, "pixi.lock")
	}

	hasOutput := false
	for _, name := range files {
		var contentA, contentB string
		if name == "pixi.toml" {
			contentA, contentB = srcA.toml, srcB.toml
		} else {
			contentA, contentB = srcA.lock, srcB.lock
		}

		diff, err := diffStrings(contentA, contentB, srcA.label+"/"+name, srcB.label+"/"+name)
		if err != nil {
			return err
		}
		if diff != "" {
			fmt.Print(diff)
			hasOutput = true
		}
	}

	if !hasOutput {
		fmt.Fprintln(os.Stderr, "No differences.")
	}
	return nil
}

// resolveSource resolves a ref (directory or workspace:tag) into a diffSource.
func resolveSource(ref, defaultLabel string) (*diffSource, error) {
	if isLocalDir(ref) {
		return resolveLocalSource(ref, defaultLabel)
	}
	return resolveServerSource(ref, defaultLabel)
}

func resolveLocalSource(dir, defaultLabel string) (*diffSource, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	label := defaultLabel
	if dir == "." {
		label = "local"
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

func resolveServerSource(ref, defaultLabel string) (*diffSource, error) {
	envName, tag := parseEnvRef(ref)

	server, err := resolveServerFlag(diffServer)
	if err != nil {
		return nil, err
	}

	client, err := getAuthenticatedClient(server)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		return nil, err
	}

	versionNumber, err := resolveVersionNumber(client, ctx, env.ID, envName, tag)
	if err != nil {
		return nil, err
	}

	toml, err := client.GetVersionPixiToml(ctx, env.ID, versionNumber)
	if err != nil {
		return nil, fmt.Errorf("fetching pixi.toml: %w", err)
	}

	lock, _ := client.GetVersionPixiLock(ctx, env.ID, versionNumber)

	label := envName
	if tag != "" {
		label = envName + ":" + tag
	}

	return &diffSource{
		label: label,
		toml:  toml,
		lock:  lock,
	}, nil
}

// resolveVersionNumber resolves a tag or latest version to a version number.
func resolveVersionNumber(client *cliclient.Client, ctx context.Context, envID, envName, tag string) (int32, error) {
	if tag != "" {
		tags, err := client.GetEnvironmentTags(ctx, envID)
		if err != nil {
			return 0, fmt.Errorf("getting tags: %w", err)
		}
		for _, t := range tags {
			if t.Tag == tag {
				return int32(t.VersionNumber), nil
			}
		}
		return 0, fmt.Errorf("tag %q not found for environment %q", tag, envName)
	}

	versions, err := client.GetEnvironmentVersions(ctx, envID)
	if err != nil {
		return 0, fmt.Errorf("getting versions: %w", err)
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("environment %q has no versions", envName)
	}
	latest := versions[0]
	for _, v := range versions {
		if v.VersionNumber > latest.VersionNumber {
			latest = v
		}
	}
	return latest.VersionNumber, nil
}

// isLocalDir returns true if ref looks like a local path.
func isLocalDir(ref string) bool {
	if strings.HasPrefix(ref, ".") || strings.HasPrefix(ref, "/") {
		return true
	}
	info, err := os.Stat(ref)
	if err == nil && info.IsDir() && !strings.Contains(ref, ":") {
		return true
	}
	return false
}

// diffStrings diffs two strings using the system diff command.
func diffStrings(a, b, labelA, labelB string) (string, error) {
	tmpA, err := writeTempFile(a)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpA)

	tmpB, err := writeTempFile(b)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpB)

	out, err := exec.Command("diff", "-u", "--label", labelA, "--label", labelB, tmpA, tmpB).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return string(out), nil
		}
		return "", fmt.Errorf("running diff: %w", err)
	}
	return "", nil
}

func writeTempFile(content string) (string, error) {
	f, err := os.CreateTemp("", "nebi-diff-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("writing temp file: %w", err)
	}
	f.Close()
	return f.Name(), nil
}
