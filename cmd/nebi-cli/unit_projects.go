package main

// Owner: U1 (projects). Also owns project.go (list/install/uninstall go
// through the SDK; install takes --version).

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

func registerProjectsCommands(root *cobra.Command) {
	var req shared.CreateProjectRequest
	var tomlFile string
	var platforms, channels, packages []string
	var createJSON bool
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project and its version 1",
		Long: `Create a project from a pixi.toml file or from flags, as version 1.

Names use letters, numbers, hyphens, underscores or dots. Without --path the
project folder goes to Nebi's default location (<data dir>/projects/<name>).
A version needs a description; it defaults to "Initial project creation".
The create job runs in the terminal; Ctrl-C cancels it.

Packages are given as name, name=constraint or name<op>version, e.g.
python=3.12.*, numpy>=2.0 or rich==13.7.1. A bare name allows any version.

Examples:
  nebi project create my-data --platform linux-64 --package python=3.12.* --package "numpy>=2.0"
  nebi project create my-data --pixi-toml ./pixi.toml --install`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Name = args[0]
			if tomlFile != "" {
				if len(platforms) > 0 || len(packages) > 0 || cmd.Flags().Changed("channel") {
					return fmt.Errorf("use either --pixi-toml or --platform/--channel/--package, not both")
				}
				data, err := os.ReadFile(tomlFile)
				if err != nil {
					return err
				}
				req.PixiToml = string(data)
			} else {
				form := &shared.ManifestForm{Name: req.Name, Platforms: platforms, Channels: channels}
				for _, p := range packages {
					name, constraint := parseProjectPackageArg(p)
					form.Requested = append(form.Requested, shared.RequestedPackage{Name: name, Constraint: constraint})
				}
				req.Form = form
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Local.CreateProject(ctx, req)
				if res != nil {
					if createJSON {
						if jerr := writeJSON(res); jerr != nil {
							return jerr
						}
					} else if err == nil {
						fmt.Fprintf(os.Stderr, "Created %s at %s\n", res.Project.Name, res.Project.Path)
						if req.Install {
							fmt.Fprintln(os.Stderr, "Version 1 is saved and installed.")
						} else {
							fmt.Fprintf(os.Stderr, "Version 1 is saved on this machine. Install it with 'nebi project install %s'.\n", res.Project.Name)
						}
					}
				}
				return err
			})
		},
	}
	create.Flags().StringVar(&req.Path, "path", "", "Where the project folder is created (default: Nebi's default location)")
	create.Flags().StringVar(&tomlFile, "pixi-toml", "", "Create from this pixi.toml instead of flags")
	create.Flags().StringSliceVar(&platforms, "platform", nil, "Platform (repeatable), e.g. linux-64")
	create.Flags().StringSliceVar(&channels, "channel", []string{"conda-forge"}, "Channel (repeatable)")
	create.Flags().StringArrayVar(&packages, "package", nil, "Package as name, name=constraint or name<op>version (repeatable)")
	create.Flags().StringVar(&req.Description, "description", "Initial project creation", "Description of version 1")
	create.Flags().StringSliceVar(&req.Tags, "tag", nil, "Tag for version 1 (repeatable); latest is added automatically")
	create.Flags().BoolVar(&req.Install, "install", false, "Install version 1 after creating it (Create and install)")
	create.Flags().BoolVar(&createJSON, "json", false, "Print the new project and its job as JSON")

	var deleteYes bool
	del := &cobra.Command{
		Use:   "delete <project>",
		Short: "Delete a local project and its versions",
		Long: `Delete a project from this machine: its installed environment, all its
versions and, when Nebi created it, its folder. A folder that existed before
the project used it is kept. Projects on a server or in a registry are not
affected.`,
		Args: cobra.ExactArgs(1),
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
				if !deleteYes {
					ok, err := confirmProjectAction(fmt.Sprintf("Delete %s and all its versions from this machine?", p.Name))
					if err != nil {
						return err
					}
					if !ok {
						fmt.Fprintln(os.Stderr, "Nothing was deleted.")
						return nil
					}
				}
				job, err := s.SDK.Local.DeleteProject(ctx, p.ID)
				if job != nil {
					fmt.Fprintf(os.Stderr, "Delete project %s: %s\n", p.Name, projectJobStatusLabel(job.Status))
				}
				return err
			})
		},
	}
	del.Flags().BoolVarP(&deleteYes, "yes", "y", false, "Do not ask for confirmation")

	var showJSON bool
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
				if showJSON {
					return writeJSON(p)
				}
				return printProjectItem(p)
			})
		},
	}
	show.Flags().BoolVar(&showJSON, "json", false, "Output as JSON")

	projectCmd.AddCommand(create, del, show)
}

// printProjectItem prints a project as aligned key/value lines.
func printProjectItem(p *shared.ProjectListItem) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	size := p.Size
	if size == "" {
		size = "-"
	}
	latest := "-"
	if p.LatestVersion != nil {
		latest = fmt.Sprintf("v%d", *p.LatestVersion)
	}
	fmt.Fprintf(w, "Name:\t%s\n", p.Name)
	fmt.Fprintf(w, "ID:\t%s\n", p.ID)
	fmt.Fprintf(w, "Path:\t%s\n", p.Path)
	fmt.Fprintf(w, "Version:\t%s\n", projectListVersion(*p))
	fmt.Fprintf(w, "Latest version:\t%s\n", latest)
	fmt.Fprintf(w, "Environment:\t%s\n", projectEnvironmentLabel(*p))
	fmt.Fprintf(w, "Size:\t%s\n", size)
	fmt.Fprintf(w, "Remotes:\t%s\n", projectRemoteNames(*p))
	return w.Flush()
}

// parseProjectPackageArg splits a --package value into name and constraint:
// "numpy" -> any version, "python=3.12.*" -> "3.12.*",
// "numpy>=2.0" or "numpy=>=2.0" -> ">=2.0", "rich==13.7" -> "==13.7".
func parseProjectPackageArg(arg string) (name, constraint string) {
	arg = strings.TrimSpace(arg)
	i := strings.IndexAny(arg, "<>=!~")
	if i < 0 {
		return arg, "*"
	}
	name = strings.TrimSpace(arg[:i])
	rest := strings.Join(strings.Fields(arg[i:]), "")
	if strings.HasPrefix(rest, "=") && !strings.HasPrefix(rest, "==") {
		rest = strings.TrimSpace(rest[1:])
	}
	if rest == "" {
		rest = "*"
	}
	return name, rest
}

// confirmProjectAction asks a yes/no question on the terminal (default no).
func confirmProjectAction(question string) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)
	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		if err.Error() == "unexpected newline" || err.Error() == "EOF" {
			return false, nil
		}
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
