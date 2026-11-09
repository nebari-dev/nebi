package main

// Owner: U3 (newversion).

import (
	"context"
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

func registerNewVersionCommands(root *cobra.Command) {
	var req shared.CreateVersionRequest
	var tomlFile string
	cmd := &cobra.Command{
		Use:   "new <project> --from <version> --pixi-toml <file>",
		Short: "Create a new version from a base version and an edited pixi.toml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(tomlFile)
			if err != nil {
				return err
			}
			req.PixiToml = string(data)
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				job, err := s.SDK.Local.CreateVersion(ctx, id, req)
				if err != nil {
					return err
				}
				if job.VersionNumber != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Version %d created\n", *job.VersionNumber)
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Create version job %s: %s\n", job.ID, job.Status)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&req.BaseVersion, "from", 0, "Base version number")
	cmd.Flags().StringVar(&tomlFile, "pixi-toml", "", "Edited pixi.toml")
	cmd.Flags().StringVar(&req.Description, "description", "", "About this version")
	cmd.Flags().StringSliceVar(&req.Tags, "tag", nil, "Tag to move to the new version (repeatable)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("pixi-toml")
	projectVersionCmd.AddCommand(cmd)
}
