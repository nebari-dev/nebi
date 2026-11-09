package main

// Owner: U4 (compare).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/spf13/cobra"
)

func registerCompareCommands(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "compare <project> <base-version> <compare-to-version>",
		Short: "Compare the requested packages of two versions (local or server-only)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			target, err := parseVersionArg(args[2])
			if err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				res, err := s.SDK.Compare(ctx, id, base, target)
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}
	projectCmd.AddCommand(cmd)
}
