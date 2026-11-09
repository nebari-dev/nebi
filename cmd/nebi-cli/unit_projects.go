package main

// Owner: U1 (projects). Also owns project.go (list/install/uninstall may be
// rewired to the SDK: install takes --version and calls SDK.Exec).

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

func registerProjectsCommands(root *cobra.Command) {
	var req shared.CreateProjectRequest
	var tomlFile string
	var platforms, channels, packages []string
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project and its version 1",
		Long: `Create a project from a pixi.toml file or from flags, as version 1.

Examples:
  nebi project create my-data --platform linux-64 --package python=3.12.* --package numpy=">=2.0"
  nebi project create my-data --pixi-toml ./pixi.toml --install`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Name = args[0]
			if tomlFile != "" {
				data, err := os.ReadFile(tomlFile)
				if err != nil {
					return err
				}
				req.PixiToml = string(data)
			} else {
				form := &shared.ManifestForm{Name: req.Name, Platforms: platforms, Channels: channels}
				for _, p := range packages {
					name, constraint, ok := strings.Cut(p, "=")
					if !ok {
						constraint = "*"
					}
					form.Requested = append(form.Requested, shared.RequestedPackage{Name: name, Constraint: constraint})
				}
				req.Form = form
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Local.CreateProject(ctx, req)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created %s (job %s: %s)\n", res.Project.Name, res.Job.ID, res.Job.Status)
				return nil
			})
		},
	}
	create.Flags().StringVar(&req.Path, "path", "", "Where the project folder is created (default: Nebi's default location)")
	create.Flags().StringVar(&tomlFile, "pixi-toml", "", "Create from this pixi.toml instead of flags")
	create.Flags().StringSliceVar(&platforms, "platform", nil, "Platform (repeatable), e.g. linux-64")
	create.Flags().StringSliceVar(&channels, "channel", []string{"conda-forge"}, "Channel (repeatable)")
	create.Flags().StringArrayVar(&packages, "package", nil, "Package as name or name=constraint (repeatable)")
	create.Flags().StringVar(&req.Description, "description", "", "Description of version 1")
	create.Flags().StringSliceVar(&req.Tags, "tag", nil, "Tag for version 1 (repeatable)")
	create.Flags().BoolVar(&req.Install, "install", false, "Install version 1 after creating it")

	del := &cobra.Command{
		Use:   "delete <project>",
		Short: "Delete a local project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				job, err := s.SDK.Local.DeleteProject(ctx, id)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Delete job %s: %s\n", job.ID, job.Status)
				return nil
			})
		},
	}

	show := &cobra.Command{
		Use:   "show <project>",
		Short: "Show a local project (installed version, status, size, remotes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				p, err := s.SDK.Local.GetProject(ctx, id)
				if err != nil {
					return err
				}
				return writeJSON(p)
			})
		},
	}

	projectCmd.AddCommand(create, del, show)
}
