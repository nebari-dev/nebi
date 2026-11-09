package main

// Owner: U7 (publish). Also owns publish.go, import.go and registry.go
// (legacy registry commands may be rewired to the SDK).

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

func registerPublishCommands(root *cobra.Command) {
	var targets []string
	publish := &cobra.Command{
		Use:   "publish-version <project> <version> --to <registry-id>=<repository>:<tag>",
		Short: "Publish a version to one or more registries",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			var req shared.PublishRequest
			for _, t := range targets {
				regRef, ref, ok := strings.Cut(t, "=")
				repo, tag, ok2 := strings.Cut(ref, ":")
				regID, err := uuid.Parse(regRef)
				if !ok || !ok2 || err != nil {
					return fmt.Errorf("invalid --to %q: use <registry-id>=<repository>:<tag>", t)
				}
				req.Targets = append(req.Targets, shared.PublishTarget{RegistryID: regID, Repository: repo, Tag: tag})
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				res, err := s.SDK.Registry.Publish(ctx, id, n, req)
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}
	publish.Flags().StringArrayVar(&targets, "to", nil, "Target as <registry-id>=<repository>:<tag> (repeatable)")

	unpublish := &cobra.Command{
		Use:   "unpublish <project> <publication-id>",
		Short: "Remove a published tag from its registry",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pubID, err := uuid.Parse(args[1])
			if err != nil {
				return fmt.Errorf("invalid publication id %q", args[1])
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				return s.SDK.Registry.Unpublish(ctx, id, pubID)
			})
		},
	}
	projectCmd.AddCommand(publish, unpublish)

	var pullReq shared.RegistryPullRequest
	pull := &cobra.Command{
		Use:   "pull <registry-id> <repository> <tag>",
		Short: "Pull a project from a registry",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			regID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid registry id %q", args[0])
			}
			pullReq.Repository, pullReq.Tag = args[1], args[2]
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Registry.Pull(ctx, regID, pullReq)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Pulled into project %s (job %s: %s)\n", res.ProjectID, res.Job.ID, res.Job.Status)
				return nil
			})
		},
	}
	pull.Flags().StringVar(&pullReq.LocalName, "name", "", "Local project name")
	pull.Flags().BoolVar(&pullReq.Install, "install", false, "Install after pulling")
	registryCmd.AddCommand(pull)
}
