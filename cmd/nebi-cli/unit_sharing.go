package main

// Owner: U6 (sharing). Share a server project from the command line: the
// same calls the client's share dialog makes (SDK remote sharing, proxied
// to the connected team server).
//
// A project argument is a server project ID or the name/ID of a local
// project linked to a server project. A principal argument is an ID, a
// username, an email, or a team name.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/spf13/cobra"
)

var shareRoleLabels = map[string]string{"owner": "Owner", "edit": "Can edit", "view": "Can view"}

func registerSharingCommands(root *cobra.Command) {
	var accessJSON bool
	access := &cobra.Command{
		Use:   "access <project>",
		Short: "Show who has access to a server project",
		Long: "Show who has access to a project on the connected server.\n\n" +
			"<project> is a server project ID, or a local project linked to one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveServerProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				res, err := s.SDK.Remote.ListAccess(ctx, id)
				if err != nil {
					return err
				}
				return printAccess(cmd.OutOrStdout(), res, accessJSON)
			})
		},
	}
	access.Flags().BoolVar(&accessJSON, "json", false, "Output as JSON")

	var role string
	var shareJSON bool
	share := &cobra.Command{
		Use:   "share <project> <user|group> <principal>",
		Short: "Give a person or team access (--role edit|view)",
		Long: "Give a person or team access to a project on the connected server.\n\n" +
			"<principal> is an ID, a username, an email or a team name.",
		Example: "  nebi server share ml-baseline user alex --role edit\n" +
			"  nebi server share ml-baseline group team-jatic",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRoleFlag(role); err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveServerProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				p, err := resolvePrincipal(ctx, s, args[1], args[2])
				if err != nil {
					return err
				}
				res, err := s.SDK.Remote.GrantAccess(ctx, id, shared.GrantAccessRequest{Principals: []shared.PrincipalRef{{Kind: p.Kind, ID: p.ID}}, Role: role})
				if err != nil {
					return err
				}
				if shareJSON {
					return writeJSON(res)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Shared with %s: %s.\n", p.Name, strings.ToLower(shareRoleLabels[role]))
				return printAccess(cmd.OutOrStdout(), res, false)
			})
		},
	}
	share.Flags().StringVar(&role, "role", "view", "Role: edit or view")
	share.Flags().BoolVar(&shareJSON, "json", false, "Output as JSON")

	var newRole string
	var setRoleJSON bool
	setRole := &cobra.Command{
		Use:   "set-role <project> <user|group> <principal> --role edit|view",
		Short: "Change a person's or team's role",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRoleFlag(newRole); err != nil {
				return err
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveServerProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				p, err := resolvePrincipal(ctx, s, args[1], args[2])
				if err != nil {
					return err
				}
				res, err := s.SDK.Remote.UpdateAccess(ctx, id, shared.PrincipalRef{Kind: p.Kind, ID: p.ID}, shared.UpdateAccessRequest{Role: newRole})
				if err != nil {
					return err
				}
				if setRoleJSON {
					return writeJSON(res)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Access updated: %s %s.\n", p.Name, strings.ToLower(shareRoleLabels[newRole]))
				return printAccess(cmd.OutOrStdout(), res, false)
			})
		},
	}
	setRole.Flags().StringVar(&newRole, "role", "", "Role: edit or view")
	setRole.Flags().BoolVar(&setRoleJSON, "json", false, "Output as JSON")
	_ = setRole.MarkFlagRequired("role")

	var unshareJSON bool
	unshare := &cobra.Command{
		Use:   "unshare <project> <user|group> <principal>",
		Short: "Remove a person's or team's access",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveServerProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				p, err := resolvePrincipal(ctx, s, args[1], args[2])
				if err != nil {
					return err
				}
				res, err := s.SDK.Remote.RevokeAccess(ctx, id, shared.PrincipalRef{Kind: p.Kind, ID: p.ID})
				if err != nil {
					return err
				}
				if unshareJSON {
					return writeJSON(res)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Access removed: %s can no longer open this project.\n", p.Name)
				return printAccess(cmd.OutOrStdout(), res, false)
			})
		},
	}
	unshare.Flags().BoolVar(&unshareJSON, "json", false, "Output as JSON")

	var principalsJSON bool
	principals := &cobra.Command{
		Use:   "principals <query>",
		Short: "Search people and teams on the server by name, email or team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.SearchPrincipals(ctx, args[0])
				if err != nil {
					return err
				}
				if principalsJSON {
					return writeJSON(res)
				}
				out := cmd.OutOrStdout()
				if len(res) == 0 {
					_, _ = fmt.Fprintln(out, "No one by that name has signed in to this server.")
					return nil
				}
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				_, _ = fmt.Fprintln(w, "KIND\tNAME\tDETAIL\tID")
				for _, p := range res {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Kind, p.Name, principalDetail(p), p.ID)
				}
				return w.Flush()
			})
		},
	}
	principals.Flags().BoolVar(&principalsJSON, "json", false, "Output as JSON")

	serverCmd.AddCommand(access, share, setRole, unshare, principals)
}

func validateRoleFlag(role string) error {
	if role != "edit" && role != "view" {
		return fmt.Errorf("invalid --role %q: use edit or view", role)
	}
	return nil
}

// principalDetail matches the share dialog's second line.
func principalDetail(p shared.Principal) string {
	if p.Kind == "group" {
		return fmt.Sprintf("Team, %d members", p.Members)
	}
	return p.Email
}

func printAccess(out io.Writer, list *shared.AccessList, asJSON bool) error {
	if asJSON {
		return writeJSON(list)
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "WHO HAS ACCESS\tDETAIL\tROLE")
	for _, e := range list.Entries {
		label := shareRoleLabels[e.Role]
		if label == "" {
			label = e.Role
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", e.Principal.Name, principalDetail(e.Principal), label)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if !list.CanManage {
		_, _ = fmt.Fprintln(out, "Only the owner or an admin can change access.")
	}
	return nil
}

// resolveServerProjectID accepts a server project UUID, or a local project
// (name or ID) linked to a server project.
func resolveServerProjectID(ctx context.Context, s *client.Stack, ref string) (string, error) {
	p, err := s.SDK.Local.ResolveProject(ctx, ref)
	if err == nil {
		var remote models.ProjectRemote
		err := s.SDK.Deps().DB.WithContext(ctx).
			Where("project_id = ? AND kind = ?", p.ID, models.RemoteKindServer).
			First(&remote).Error
		if err == nil && remote.ServerProjectID != "" {
			return remote.ServerProjectID, nil
		}
		if _, perr := uuid.Parse(ref); perr != nil {
			return "", fmt.Errorf("project %q is not on the server yet; push it first with 'nebi project push %s'", ref, ref)
		}
	} else if !errors.Is(err, shared.ErrNotFound) {
		return "", err
	}
	if _, err := uuid.Parse(ref); err != nil {
		return "", fmt.Errorf("project %q not found: pass a server project ID or a local project linked to the server", ref)
	}
	return ref, nil
}

// resolvePrincipal turns an ID, username, email or team name into a
// principal by searching the server; names must match exactly.
func resolvePrincipal(ctx context.Context, s *client.Stack, kind, ref string) (shared.Principal, error) {
	switch kind {
	case "team":
		kind = "group"
	case "user", "group":
	default:
		return shared.Principal{}, fmt.Errorf("invalid kind %q: use user or group", kind)
	}
	if _, err := uuid.Parse(ref); err == nil {
		return shared.Principal{Kind: kind, ID: ref, Name: ref}, nil
	}
	found, err := s.SDK.Remote.SearchPrincipals(ctx, ref)
	if err != nil {
		return shared.Principal{}, err
	}
	for _, p := range found {
		if p.Kind != kind {
			continue
		}
		if strings.EqualFold(p.Name, ref) || (p.Email != "" && strings.EqualFold(p.Email, ref)) {
			return p, nil
		}
	}
	noun := "user"
	if kind == "group" {
		noun = "team"
	}
	return shared.Principal{}, fmt.Errorf("no %s named %q on the server; try 'nebi server principals %s'", noun, ref, ref)
}
