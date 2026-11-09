package main

// Owner: U5 (remote). Also owns push.go, pull.go, login.go, logout.go
// (legacy commands; keep them working).

import (
	"context"
	"fmt"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

func registerRemoteCommands(root *cobra.Command) {
	var token string
	connect := &cobra.Command{
		Use:   "connect <server-url>",
		Short: "Connect to a team server (one server at a time)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := shared.ConnectRequest{URL: args[0], Auth: "none"}
			if token != "" {
				req.Auth, req.Token = "token", token
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				conn, err := s.SDK.Remote.Connect(ctx, req)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Connected to %s as %s\n", conn.URL, conn.Username)
				return nil
			})
		},
	}
	connect.Flags().StringVar(&token, "token", "", "API token (omit for no authentication)")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show the server connection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				conn, err := s.SDK.Remote.GetConnection(ctx)
				if err != nil {
					return err
				}
				return writeJSON(conn)
			})
		},
	}

	disconnect := &cobra.Command{
		Use:   "disconnect",
		Short: "Disconnect from the server (pulled projects stay and keep working)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				return s.SDK.Remote.Disconnect(ctx)
			})
		},
	}

	var search string
	var refresh bool
	projects := &cobra.Command{
		Use:   "projects",
		Short: "List and search server projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.ListServerProjects(ctx, search, refresh)
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}
	projects.Flags().StringVar(&search, "search", "", "Filter by name")
	projects.Flags().BoolVar(&refresh, "refresh", false, "Refresh from the server")

	var pullReq shared.PullRequest
	pull := &cobra.Command{
		Use:   "pull <server-project-id>",
		Short: "Pull a server project to this machine (optionally installing it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.Pull(ctx, args[0], pullReq)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Pulled into project %s (job %s: %s)\n", res.ProjectID, res.Job.ID, res.Job.Status)
				return nil
			})
		},
	}
	pull.Flags().StringVar(&pullReq.LocalName, "name", "", "Local name (default: the server name)")
	pull.Flags().StringVar(&pullReq.Path, "path", "", "Where the project folder is created")
	pull.Flags().BoolVar(&pullReq.Install, "install", false, "Install the latest version after pulling")

	serverCmd.AddCommand(connect, status, disconnect, projects, pull)

	push := &cobra.Command{
		Use:   "push <project>",
		Short: "Push local versions to the connected server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				res, err := s.SDK.Remote.Push(ctx, id)
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}

	pullVersion := &cobra.Command{
		Use:   "pull-version <project> <version>",
		Short: "Pull a version that only exists on the server",
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
				v, err := s.SDK.Remote.PullVersion(ctx, id, n)
				if err != nil {
					return err
				}
				return writeJSON(v)
			})
		},
	}
	projectCmd.AddCommand(push, pullVersion)
}
