package main

// Owner: U2 (detail). Also owns project_version.go (list/show may be
// rewired to the SDK).
//
// Commands (all local, through the client SDK):
//   nebi project versions <project> [--json]
//   nebi project file <project> <version> pixi.toml|pixi.lock
//   nebi project packages <project> <version> [--platform P] [--json]
//   nebi project archive <project> <version> [-o file]
//   nebi project version install <project> <version>
//   nebi project version uninstall <project>

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/utils"
	"github.com/spf13/cobra"
)

func registerDetailCommands(root *cobra.Command) {
	var versionsJSON bool
	versions := &cobra.Command{
		Use:   "versions <project>",
		Short: "List a project's versions (author, description, tags, published, sync)",
		Long: `List a project's versions, newest first, with their tags, author,
description, where they are published, and how they relate to the
connected server (newest here, newest on server, in sync).

Examples:
  nebi project versions my-project
  nebi project versions my-project --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				items, err := s.SDK.Local.ListVersions(ctx, id)
				if err != nil {
					return err
				}
				if versionsJSON {
					return writeJSON(items)
				}
				return printVersionTable(cmd.OutOrStdout(), items)
			})
		},
	}
	versions.Flags().BoolVar(&versionsJSON, "json", false, "Output as JSON")

	files := &cobra.Command{
		Use:   "file <project> <version> <pixi.toml|pixi.lock>",
		Short: "Print a version's pixi.toml or pixi.lock",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				f, err := s.SDK.Local.GetVersionFile(ctx, id, n, args[2])
				if err != nil {
					return err
				}
				out := f.Content
				if out != "" && !strings.HasSuffix(out, "\n") {
					out += "\n"
				}
				_, err = fmt.Fprint(cmd.OutOrStdout(), out)
				return err
			})
		},
	}

	var packagesJSON bool
	var packagesPlatform string
	packages := &cobra.Command{
		Use:   "packages <project> <version>",
		Short: "List resolved packages per platform",
		Long: `List the packages pixi.lock resolved for a version: name, version,
channel and download size. Shows this machine's platform when the version
supports it, else the first platform; pick another with --platform.

Examples:
  nebi project packages my-project 3
  nebi project packages my-project 3 --platform linux-64
  nebi project packages my-project 3 --json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				p, err := s.SDK.Local.ResolvedPackages(ctx, id, n)
				if err != nil {
					return err
				}
				if packagesJSON {
					return writeJSON(p)
				}
				return printResolvedPackages(cmd.OutOrStdout(), p, packagesPlatform)
			})
		},
	}
	packages.Flags().BoolVar(&packagesJSON, "json", false, "Output as JSON")
	packages.Flags().StringVar(&packagesPlatform, "platform", "", "Platform to list (default: this machine, else the first)")

	var out string
	archive := &cobra.Command{
		Use:   "archive <project> <version>",
		Short: "Download a version's files as a .tar.gz",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				dir := "."
				if out != "" {
					dir = filepath.Dir(out)
				}
				f, err := os.CreateTemp(dir, ".nebi-archive-*")
				if err != nil {
					return err
				}
				name, werr := s.SDK.Local.WriteVersionArchive(ctx, id, n, f)
				cerr := f.Close()
				if werr != nil {
					os.Remove(f.Name())
					return werr
				}
				if cerr != nil {
					os.Remove(f.Name())
					return cerr
				}
				target := out
				if target == "" {
					target = name
				}
				if err := os.Chmod(f.Name(), 0o644); err != nil {
					os.Remove(f.Name())
					return err
				}
				if err := os.Rename(f.Name(), target); err != nil {
					os.Remove(f.Name())
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", target)
				return nil
			})
		},
	}
	archive.Flags().StringVarP(&out, "output", "o", "", "Output file (default: <project>-v<N>.tar.gz)")

	install := &cobra.Command{
		Use:   "install <project> <version>",
		Short: "Install a version (replaces the installed version)",
		Long: `Install one version of a project on this machine. One version is
installed at a time: installing another version replaces the current one.
The version's pixi.toml and pixi.lock are written to the project directory
and installed exactly as locked (pixi install --frozen).

Example:
  nebi project version install my-project 5`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				job, err := s.SDK.Exec.InstallVersion(ctx, id, shared.InstallRequest{Version: &n})
				if err != nil {
					if job != nil {
						return fmt.Errorf("version %d couldn't install (job %s): %w", n, job.ID, err)
					}
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Version %d installed · ready to use on this machine\n", n)
				return nil
			})
		},
	}

	uninstall := &cobra.Command{
		Use:   "uninstall <project>",
		Short: "Uninstall the installed version",
		Long: `Remove the installed environment from disk. The project and all its
versions stay, and Nebi remembers the version so you can install it again.

Example:
  nebi project version uninstall my-project`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				job, err := s.SDK.Exec.Uninstall(ctx, id)
				if err != nil {
					return err
				}
				if job.VersionNumber != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Version %d uninstalled\n", *job.VersionNumber)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "Environment uninstalled")
				}
				return nil
			})
		},
	}

	projectCmd.AddCommand(versions, files, packages, archive)
	projectVersionCmd.AddCommand(install, uninstall)
}

// printVersionTable renders the version rail as a table.
func printVersionTable(w io.Writer, items []shared.VersionItem) error {
	if len(items) == 0 {
		fmt.Fprintln(w, "No versions yet.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSION\tSTATUS\tTAGS\tAUTHOR\tCREATED\tDESCRIPTION")
	for _, v := range items {
		fmt.Fprintf(tw, "v%d\t%s\t%s\t%s\t%s\t%s\n",
			v.Number,
			versionStatus(v),
			strings.Join(v.Tags, ", "),
			v.Author.Name,
			v.CreatedAt.Local().Format("2006-01-02 15:04"),
			v.Description,
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, v := range items {
		for _, p := range v.Published {
			fmt.Fprintf(w, "v%d published to %s\n", v.Number, p.Reference)
		}
	}
	return nil
}

// versionStatus summarizes installed / sync state for the table.
func versionStatus(v shared.VersionItem) string {
	var parts []string
	if v.Installed {
		parts = append(parts, "installed")
	}
	if v.ServerOnly {
		parts = append(parts, "only on server")
	}
	if v.Sync != nil {
		switch *v.Sync {
		case shared.SyncNewestHere:
			parts = append(parts, "newest here")
		case shared.SyncNewestOnServer:
			parts = append(parts, "newest on server")
		case shared.SyncInSync:
			parts = append(parts, "in sync")
		}
	}
	if len(v.Published) > 0 {
		parts = append(parts, "published")
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

// printResolvedPackages renders one platform's resolved packages.
func printResolvedPackages(w io.Writer, p *shared.ResolvedPackages, platform string) error {
	if len(p.Platforms) == 0 {
		fmt.Fprintln(w, "This version has no pixi.lock, so nothing is resolved yet.")
		return nil
	}
	if platform == "" {
		platform = p.Platforms[0]
		if here := artifact.DefaultPlatform(runtime.GOOS, runtime.GOARCH); slices.Contains(p.Platforms, here) {
			platform = here
		}
	} else if !slices.Contains(p.Platforms, platform) {
		return fmt.Errorf("platform %q is not in this version's pixi.lock (have: %s)", platform, strings.Join(p.Platforms, ", "))
	}
	pkgs := p.Packages[platform]
	fmt.Fprintf(w, "Platform: %s (%d packages, %s to download)\n\n",
		artifact.PlatformDisplayName(platform, artifact.DefaultPlatform(runtime.GOOS, runtime.GOARCH)),
		len(pkgs), utils.FormatBytes(p.TotalSize[platform]))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tVERSION\tCHANNEL\tDOWNLOAD SIZE")
	for _, pkg := range pkgs {
		size := "-"
		if pkg.Size > 0 {
			size = utils.FormatBytes(pkg.Size)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", pkg.Name, pkg.Version, pkg.Channel, size)
	}
	return tw.Flush()
}
