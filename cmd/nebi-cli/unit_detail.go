package main

// Owner: U2 (detail). Also owns project_version.go (list/show may be
// rewired to the SDK).

import (
	"context"
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/spf13/cobra"
)

func registerDetailCommands(root *cobra.Command) {
	versions := &cobra.Command{
		Use:   "versions <project>",
		Short: "List a project's versions (author, description, tags, published, sync)",
		Args:  cobra.ExactArgs(1),
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
				return writeJSON(items)
			})
		},
	}

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
				_, err = fmt.Fprint(cmd.OutOrStdout(), f.Content)
				return err
			})
		},
	}

	packages := &cobra.Command{
		Use:   "packages <project> <version>",
		Short: "List resolved packages per platform",
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
				p, err := s.SDK.Local.ResolvedPackages(ctx, id, n)
				if err != nil {
					return err
				}
				return writeJSON(p)
			})
		},
	}

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
				f, err := os.CreateTemp(".", ".nebi-archive-*")
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
					return cerr
				}
				target := out
				if target == "" {
					target = name
				}
				if err := os.Rename(f.Name(), target); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", target)
				return nil
			})
		},
	}
	archive.Flags().StringVarP(&out, "output", "o", "", "Output file (default: suggested name)")

	projectCmd.AddCommand(versions, files, packages, archive)
}
