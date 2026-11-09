package main

// Owner: U6 (sharing).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

func registerSharingCommands(root *cobra.Command) {
	access := &cobra.Command{
		Use:   "access <server-project-id>",
		Short: "Show who has access to a server project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.ListAccess(ctx, args[0])
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}

	var role string
	share := &cobra.Command{
		Use:   "share <server-project-id> <user|group> <principal-id>",
		Short: "Give a person or team access (--role edit|view)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := shared.GrantAccessRequest{Principals: []shared.PrincipalRef{{Kind: args[1], ID: args[2]}}, Role: role}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.GrantAccess(ctx, args[0], req)
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}
	share.Flags().StringVar(&role, "role", "view", "Role: edit or view")

	var newRole string
	setRole := &cobra.Command{
		Use:   "set-role <server-project-id> <user|group> <principal-id> --role edit|view",
		Short: "Change a person's or team's role",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.UpdateAccess(ctx, args[0], shared.PrincipalRef{Kind: args[1], ID: args[2]}, shared.UpdateAccessRequest{Role: newRole})
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}
	setRole.Flags().StringVar(&newRole, "role", "", "Role: edit or view")
	_ = setRole.MarkFlagRequired("role")

	unshare := &cobra.Command{
		Use:   "unshare <server-project-id> <user|group> <principal-id>",
		Short: "Remove a person's or team's access",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.RevokeAccess(ctx, args[0], shared.PrincipalRef{Kind: args[1], ID: args[2]})
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}

	principals := &cobra.Command{
		Use:   "principals <query>",
		Short: "Search people and teams on the server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.SearchPrincipals(ctx, args[0])
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}

	serverCmd.AddCommand(access, share, setRole, unshare, principals)
}
