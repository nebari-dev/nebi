package main

// Owner: U4 (compare).

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

func registerCompareCommands(root *cobra.Command) {
	var asJSON, resolved bool
	cmd := &cobra.Command{
		Use:   "compare <project> <base-version> <compare-to-version>",
		Short: "Compare the requested packages of two versions (local or server-only)",
		Long: `Compare two versions of a local project, as the Compare versions page does.

Rows list the union of requested packages: "+" added, "-" removed, "~" changed
(constraint or resolved version differs). Versions shown are the resolved
versions from pixi.lock for this machine's platform when the lock has it,
falling back to the requested constraint.

Either version may exist only on the connected Nebi server; it is fetched
from there without being pulled.

Examples:
  nebi project compare data-science 3 5
  nebi project compare data-science 5 7 --resolved
  nebi project compare data-science 5 7 --json`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			target, err := parseVersionArg(args[2])
			if err != nil {
				return err
			}
			// The arguments are valid: a failure from here on (unknown
			// version, server unreachable) is not a usage problem.
			cmd.SilenceUsage = true
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				res, err := s.SDK.Compare(ctx, id, base, target)
				if err != nil {
					return err
				}
				if asJSON {
					return writeJSON(res)
				}
				writeCompareResult(cmd.OutOrStdout(), res, resolved)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&resolved, "resolved", false, "Also list every resolved package from pixi.lock")
	projectCmd.AddCommand(cmd)
}

// printPackageComparison is "nebi diff --packages": the Compare versions
// view of two diff sources (directories, tracked projects or server refs).
func printPackageComparison(w io.Writer, a, b *diffSource, withResolved bool) error {
	sideA := artifact.VersionContent{Toml: []byte(a.toml), Lock: []byte(a.lock)}
	sideB := artifact.VersionContent{Toml: []byte(b.toml), Lock: []byte(b.lock)}
	d, err := artifact.DiffVersions(sideA, sideB, artifact.DiffOptions{Platform: machinePlatformIn(sideA, sideB)})
	if err != nil {
		return fmt.Errorf("comparing packages: %w", err)
	}
	writeVersionDiff(w, a.label, b.label, d, withResolved)
	return nil
}

// machinePlatformIn returns this machine's pixi platform when either lock
// resolves it, else "" (artifact.DiffVersions then picks one). Same rule
// as the SDK's Compare.
func machinePlatformIn(sides ...artifact.VersionContent) string {
	mp := artifact.DefaultPlatform(runtime.GOOS, runtime.GOARCH)
	if mp == "" {
		return ""
	}
	for _, s := range sides {
		l, err := artifact.ParseLock(s.Lock)
		if err != nil {
			continue
		}
		if env := l.Environment(""); env != nil {
			if _, ok := env.Packages[mp]; ok {
				return mp
			}
		}
	}
	return ""
}

// compareVersionLabel mirrors the UI picker labels: "v7 (installed, latest
// on server)".
func compareVersionLabel(ref shared.CompareVersionRef) string {
	var notes []string
	if ref.Installed {
		notes = append(notes, "installed")
	}
	if ref.ServerLatest {
		notes = append(notes, "latest on server")
	} else if ref.ServerOnly {
		notes = append(notes, "on server")
	}
	if len(notes) == 0 {
		return fmt.Sprintf("v%d", ref.Number)
	}
	return fmt.Sprintf("v%d (%s)", ref.Number, strings.Join(notes, ", "))
}

// writeCompareResult prints a compare result as text.
func writeCompareResult(w io.Writer, res *shared.CompareResult, resolved bool) {
	writeVersionDiff(w, compareVersionLabel(res.Base), compareVersionLabel(res.Target), &res.VersionDiff, resolved)
}

// writeVersionDiff prints an artifact.VersionDiff: summary line, channel
// and platform changes, the requested-package table and optionally the
// resolved-package table. Shared by "nebi project compare" and
// "nebi diff --packages".
func writeVersionDiff(w io.Writer, baseLabel, targetLabel string, d *artifact.VersionDiff, resolved bool) {
	fmt.Fprintf(w, "Compare versions: %s -> %s\n", baseLabel, targetLabel)
	channels, platforms := formatListDiff(d.Channels), formatListDiff(d.Platforms)
	summary := formatDiffCounts(d.RequestedCounts)
	if !d.RequestedCounts.HasChanges() && (channels != "" || platforms != "") {
		// Only channels or platforms differ: the versions are not the same.
		summary = "+0 -0 ~0"
	}
	fmt.Fprintf(w, "Summary of changes: %s\n", summary)
	if channels != "" {
		fmt.Fprintf(w, "Channels: %s\n", channels)
	}
	if platforms != "" {
		fmt.Fprintf(w, "Platforms: %s\n", platforms)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Requested packages")
	writeDiffTable(w, d.Requested, true)

	if resolved {
		fmt.Fprintln(w)
		platform := d.Platform
		if platform == "" {
			platform = "no lock"
		}
		fmt.Fprintf(w, "Resolved packages (%s): %s\n", platform, formatDiffCounts(d.ResolvedCounts))
		writeDiffTable(w, d.Resolved, false)
	}
}

// formatDiffCounts is "+1 -2 ~0" or "No changes".
func formatDiffCounts(c artifact.DiffCounts) string {
	if !c.HasChanges() {
		return "No changes"
	}
	return fmt.Sprintf("+%d -%d ~%d", c.Added, c.Removed, c.Changed)
}

func formatListDiff(l artifact.ListDiff) string {
	var parts []string
	for _, a := range l.Added {
		parts = append(parts, "+"+a)
	}
	for _, r := range l.Removed {
		parts = append(parts, "-"+r)
	}
	return strings.Join(parts, " ")
}

var diffMarkers = map[artifact.Change]string{
	artifact.ChangeAdded:   "+",
	artifact.ChangeRemoved: "-",
	artifact.ChangeChanged: "~",
	artifact.ChangeSame:    " ",
}

func writeDiffTable(w io.Writer, rows []artifact.PackageDiffEntry, requested bool) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, " \tPACKAGE\tBASE\tCOMPARE TO")
	for _, r := range rows {
		name := r.Name
		if r.Source == artifact.SourcePyPI {
			name += " (pypi)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", diffMarkers[r.Change], name, packageRefText(r.Base, requested), packageRefText(r.Compare, requested))
	}
	_ = tw.Flush()
}

// packageRefText is the version shown for one side; requested rows add the
// constraint when it differs from the resolved version.
func packageRefText(ref *artifact.PackageRef, requested bool) string {
	if ref == nil {
		return "-"
	}
	out := ref.Version
	if !requested && ref.Build != "" {
		out += " " + ref.Build
	}
	if requested && ref.Constraint != "" && ref.Constraint != ref.Version {
		out += " (" + ref.Constraint + ")"
	}
	return out
}
