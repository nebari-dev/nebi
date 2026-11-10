package main

// Owner: U7 (publish). Also owns publish.go, import.go and registry.go
// (their local paths run on the client SDK).

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/registry"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func registerPublishCommands(root *cobra.Command) {
	var targets []string
	var publishJSON bool
	publish := &cobra.Command{
		Use:   "publish-version <project> <version> [--to <registry>[=<repository>:<tag>]]...",
		Short: "Publish a version to one or more registries",
		Long: `Publish one version of a local project to one or more OCI registries.

Each --to names a registry (name or ID). Add =<repository>:<tag> to choose
where it goes; without it the registry's default (<namespace>/<project>:<version>)
is used. Without --to the version goes to the default registry.

Examples:
  nebi project publish-version Project_1 5
  nebi project publish-version Project_1 5 --to quay --to ghcr=example-org/project_1:stable`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLocalFlags(cmd)
			n, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				req, err := publishTargets(ctx, s, id, n, targets)
				if err != nil {
					return err
				}
				res, err := s.SDK.Registry.Publish(ctx, id, n, req)
				if err != nil {
					return err
				}
				if publishJSON {
					if err := writeJSON(res); err != nil {
						return err
					}
				} else {
					printPublishResults(cmd.OutOrStdout(), n, res)
				}
				for _, r := range res.Results {
					if r.Status != registry.StatusPublished {
						return fmt.Errorf("version %d was not published to every registry", n)
					}
				}
				return nil
			})
		},
	}
	publish.Flags().StringArrayVar(&targets, "to", nil, "Registry name or ID, optionally =<repository>:<tag> (repeatable)")
	publish.Flags().BoolVar(&publishJSON, "json", false, "Output the per-registry results as JSON")

	unpublish := &cobra.Command{
		Use:   "unpublish <project> <publication-id>",
		Short: "Remove a published tag from its registry",
		Long: `Remove a published tag from its registry. Pulls of this tag will stop working.

Find publication IDs with 'nebi project publications <project>'.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLocalFlags(cmd)
			pubID, err := uuid.Parse(args[1])
			if err != nil {
				return fmt.Errorf("invalid publication id %q", args[1])
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				refs, err := s.SDK.Registry.PublishedRefs(ctx, id)
				if err != nil {
					return err
				}
				if err := s.SDK.Registry.Unpublish(ctx, id, pubID); err != nil {
					return err
				}
				for n, list := range refs {
					for _, r := range list {
						if r.PublicationID != nil && *r.PublicationID == pubID {
							fmt.Fprintf(cmd.ErrOrStderr(), "Version %d unpublished from %s\n", n, r.Registry)
							return nil
						}
					}
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Unpublished")
				return nil
			})
		},
	}

	var pubsJSON bool
	publications := &cobra.Command{
		Use:   "publications <project>",
		Short: "List where a project's versions are published",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLocalFlags(cmd)
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				refs, err := s.SDK.Registry.PublishedRefs(ctx, id)
				if err != nil {
					return err
				}
				versions := make([]int, 0, len(refs))
				for n := range refs {
					versions = append(versions, n)
				}
				sort.Sort(sort.Reverse(sort.IntSlice(versions)))
				if pubsJSON {
					type row struct {
						Version int `json:"version"`
						shared.PublishedRef
					}
					out := []row{}
					for _, n := range versions {
						for _, r := range refs[n] {
							out = append(out, row{Version: n, PublishedRef: r})
						}
					}
					return writeJSON(out)
				}
				if len(versions) == 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "No versions of this project are published from this machine.")
					return nil
				}
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "VERSION\tPUBLISHED TO\tPUBLICATION ID\tUNPUBLISH")
				for _, n := range versions {
					for _, r := range refs[n] {
						can := "yes"
						if !r.CanUnpublish {
							can = "no"
						}
						fmt.Fprintf(w, "v%d\t%s\t%s\t%s\n", n, r.Reference, r.PublicationID, can)
					}
				}
				return w.Flush()
			})
		},
	}
	publications.Flags().BoolVar(&pubsJSON, "json", false, "Output as JSON")
	projectCmd.AddCommand(publish, unpublish, publications)

	var pullReq shared.RegistryPullRequest
	pull := &cobra.Command{
		Use:   "pull <registry> <repository> <tag>",
		Short: "Pull a project from a registry into a new local project",
		Long: `Pull a Nebi project from an OCI registry. It becomes a new local project
in Nebi's projects folder, with the artifact as version 1.

Examples:
  nebi registry pull quay nebari/ml-baseline 5
  nebi registry pull quay nebari/ml-baseline 5 --name ml --install`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLocalFlags(cmd)
			pullReq.Repository, pullReq.Tag = args[1], args[2]
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				reg, err := s.SDK.Registry.ResolveRegistry(ctx, args[0])
				if err != nil {
					return err
				}
				res, err := s.SDK.Registry.Pull(ctx, reg.ID, pullReq)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Pulled %s:%s into project %s (%s)\n", pullReq.Repository, pullReq.Tag, res.Job.ProjectName, res.ProjectID)
				return nil
			})
		},
	}
	pull.Flags().StringVar(&pullReq.LocalName, "name", "", "Local project name (defaults to the repository name)")
	pull.Flags().BoolVar(&pullReq.Install, "install", false, "Install after pulling")

	var repoSearch string
	repos := &cobra.Command{
		Use:   "repos <registry>",
		Short: "List repositories in a registry's namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLocalFlags(cmd)
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				reg, err := s.SDK.Registry.ResolveRegistry(ctx, args[0])
				if err != nil {
					return err
				}
				list, err := s.SDK.Registry.ListRepositories(ctx, reg.ID, repoSearch)
				if err != nil {
					return err
				}
				for _, r := range list {
					fmt.Fprintln(cmd.OutOrStdout(), r)
				}
				return nil
			})
		},
	}
	repos.Flags().StringVar(&repoSearch, "search", "", "Only repositories whose name contains this text")

	tags := &cobra.Command{
		Use:   "tags <registry> <repository>",
		Short: "List tags of a repository",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLocalFlags(cmd)
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				reg, err := s.SDK.Registry.ResolveRegistry(ctx, args[0])
				if err != nil {
					return err
				}
				list, err := s.SDK.Registry.ListTags(ctx, reg.ID, args[1])
				if err != nil {
					return err
				}
				for _, t := range list {
					fmt.Fprintln(cmd.OutOrStdout(), t)
				}
				return nil
			})
		},
	}
	registryCmd.AddCommand(pull, repos, tags, newRegistryUpdateCmd())
}

// publishTargets builds the publish request from --to values, filling
// repository:tag from the publish defaults when only a registry is named.
func publishTargets(ctx context.Context, s *client.Stack, projectID uuid.UUID, version int, to []string) (shared.PublishRequest, error) {
	var req shared.PublishRequest
	defaults, err := s.SDK.Registry.PublishDefaults(ctx, projectID, version)
	if err != nil {
		return req, err
	}
	if len(to) == 0 {
		to = []string{""}
	}
	for _, t := range to {
		regRef, ref, hasRef := strings.Cut(t, "=")
		reg, err := s.SDK.Registry.ResolveRegistry(ctx, regRef)
		if err != nil {
			return req, err
		}
		target := shared.PublishTarget{RegistryID: reg.ID}
		if hasRef {
			if target.Repository, target.Tag, err = registry.ParseReference(ref); err != nil {
				return req, fmt.Errorf("invalid --to %q: %w", t, err)
			}
		} else {
			for _, d := range defaults {
				if d.RegistryID == reg.ID {
					target.Repository, target.Tag = d.Repository, d.Tag
				}
			}
		}
		req.Targets = append(req.Targets, target)
	}
	return req, nil
}

func printPublishResults(out io.Writer, version int, res *shared.PublishResult) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REGISTRY\tREFERENCE\tRESULT")
	for _, r := range res.Results {
		status := "Published"
		if r.Status != registry.StatusPublished {
			status = "Failed: " + r.Error
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.RegistryName, r.Reference, status)
	}
	_ = w.Flush()
	var ok []string
	for _, r := range res.Results {
		if r.Status == registry.StatusPublished {
			ok = append(ok, r.RegistryName)
		}
	}
	if len(ok) > 0 {
		fmt.Fprintf(out, "Version %d published to %s\n", version, strings.Join(ok, ", "))
	}
}

// resetLocalFlags puts the command's own flags back to their defaults
// after a run. The flags of these commands bind closure variables (not the
// package globals the in-process test harness resets), and cobra keeps
// flag state between Execute calls, so a repeated --to would otherwise
// accumulate across invocations in one process.
func resetLocalFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

// resetFlagsByName resets only the named flags (for commands whose other
// flags are package globals).
func resetFlagsByName(cmd *cobra.Command, names ...string) {
	for _, n := range names {
		if f := cmd.Flags().Lookup(n); f != nil {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
}
