package main

// Owner: U5 (remote). Also owns push.go, pull.go, login.go, logout.go
// (legacy commands; keep them working).
//
// SDK-backed server commands (CLI parity for internal/client/sdk/remote):
//
//	nebi server connect <url> [--token T]
//	nebi server status [--json]
//	nebi server disconnect
//	nebi server projects [--search S] [--refresh] [--json]
//	nebi server show <server-project-id> [--json]
//	nebi server pull <server-project-id> [--name N] [--path P] [--install]
//	nebi project push <project> [--up-to N] [--json]
//	nebi project pull-version <project> <version> [--json]
//
// `nebi status` (legacy status.go) also prints the SDK's version sync for
// the tracked project in the current directory (projectVersionSync).

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/spf13/cobra"
)

func registerRemoteCommands(root *cobra.Command) {
	serverCmds := []*cobra.Command{
		newServerConnectCmd(),
		newServerStatusCmd(),
		newServerDisconnectCmd(),
		newServerProjectsCmd(),
		newServerShowCmd(),
		newServerPullCmd(),
	}
	projectCmds := []*cobra.Command{newProjectPushCmd(), newProjectPullVersionCmd()}
	for _, c := range append(append([]*cobra.Command{}, serverCmds...), projectCmds...) {
		silenceUsageOnRunErrors(c)
	}
	serverCmd.AddCommand(serverCmds...)
	projectCmd.AddCommand(projectCmds...)
}

// silenceUsageOnRunErrors keeps cobra from printing the usage text when
// the command itself fails (server unreachable, name taken, ...); argument
// errors still show it.
func silenceUsageOnRunErrors(c *cobra.Command) {
	run := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		return run(cmd, args)
	}
}

func newServerConnectCmd() *cobra.Command {
	var token string
	var tokenStdin bool
	cmd := &cobra.Command{
		Use:   "connect <server-url>",
		Short: "Connect to a team server (one server at a time)",
		Long: `Connect this machine to a Nebi team server. You can connect to one
server at a time; connecting to another server replaces the connection.

Authentication is a token (--token or --token-stdin) or none.

Examples:
  nebi server connect nebi.example.com --token <api-token>
  echo "$TOKEN" | nebi server connect https://nebi.example.com --token-stdin
  nebi server connect http://localhost:8460`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if tokenStdin {
				if token != "" {
					return fmt.Errorf("use either --token or --token-stdin")
				}
				data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
				if err != nil {
					return fmt.Errorf("reading token: %w", err)
				}
				token = strings.TrimSpace(string(data))
			}
			req := shared.ConnectRequest{URL: args[0], Auth: "none"}
			if token != "" {
				req.Auth, req.Token = "token", token
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				conn, err := s.SDK.Remote.Connect(ctx, req)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Connected to %s as %s\n", displayServerURL(conn.URL), conn.Username)
				fmt.Fprintln(cmd.OutOrStdout(), "Projects shared with you are listed by 'nebi server projects'.")
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "API token (omit for no authentication)")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read the token from stdin")
	return cmd
}

func newServerStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the server connection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				conn, err := s.SDK.Remote.GetConnection(ctx)
				if err != nil {
					return err
				}
				if asJSON {
					return writeJSON(conn)
				}
				out := cmd.OutOrStdout()
				if !conn.Connected {
					fmt.Fprintln(out, "Not connected. Run 'nebi server connect <url>' to connect a team server.")
					return nil
				}
				fmt.Fprintf(out, "Connected to  %s\n", displayServerURL(conn.URL))
				fmt.Fprintf(out, "Signed in as  %s\n", conn.Username)
				fmt.Fprintf(out, "Last synced   %s\n", syncedAgo(conn.LastSyncedAt))
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newServerDisconnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disconnect",
		Short: "Disconnect from the server (pulled projects stay and keep working)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				conn, err := s.SDK.Remote.GetConnection(ctx)
				if err != nil {
					return err
				}
				if !conn.Connected {
					fmt.Fprintln(cmd.OutOrStdout(), "Not connected.")
					return nil
				}
				if err := s.SDK.Remote.Disconnect(ctx); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Disconnected from %s. Projects you pulled stay on this machine and keep working.\n", displayServerURL(conn.URL))
				return nil
			})
		},
	}
}

func newServerProjectsCmd() *cobra.Command {
	var search string
	var refresh, asJSON bool
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List and search server projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.ListServerProjects(ctx, search, refresh)
				if err != nil {
					return err
				}
				if asJSON {
					return writeJSON(res)
				}
				out := cmd.OutOrStdout()
				if len(res.Projects) == 0 {
					if search != "" {
						fmt.Fprintf(out, "No server projects match %q.\n", search)
					} else {
						fmt.Fprintln(out, "Nothing is shared with you on this server yet.")
					}
					return nil
				}
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "PROJECT\tLATEST\tNAMESPACE\tROLE\tON THIS MACHINE\tID")
				for _, p := range res.Projects {
					latest := "-"
					if p.LatestVersion != nil {
						latest = fmt.Sprintf("v%d", *p.LatestVersion)
					}
					here := "no"
					if p.LocalProjectID != nil {
						here = "yes"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, latest, p.Namespace, p.Role, here, p.ID)
				}
				return w.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "Filter by name")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Refresh from the server")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newServerShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <server-project-id>",
		Short: "Show a server project and its versions (remote details)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				d, err := s.SDK.Remote.GetRemoteDetails(ctx, args[0])
				if err != nil {
					return err
				}
				if asJSON {
					return writeJSON(d)
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "%s (%s, owner %s, your role: %s)\n", d.Project.Name, d.Project.Namespace, d.Project.Owner, d.Project.Role)
				fmt.Fprintf(out, "%s\n\n", d.URL)
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "VERSION\tTAGS\tAUTHOR\tON THIS MACHINE\tDESCRIPTION")
				for _, v := range d.Versions {
					here := "yes"
					if v.ServerOnly {
						here = "no"
					}
					fmt.Fprintf(w, "v%d\t%s\t%s\t%s\t%s\n", v.Number, strings.Join(v.Tags, ","), v.Author.Name, here, v.Description)
				}
				return w.Flush()
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newServerPullCmd() *cobra.Command {
	var req shared.PullRequest
	cmd := &cobra.Command{
		Use:   "pull <server-project-id>",
		Short: "Pull a server project to this machine (optionally installing it)",
		Long: `Pull every version of a server project to this machine. The project keeps
the server's version numbers. With --install the latest version is
installed after pulling.

Examples:
  nebi server pull 4b1c... --name ml-baseline
  nebi server pull 4b1c... --install`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.SDK.Remote.Pull(ctx, args[0], req)
				if res != nil && res.Job.Status == "cancelled" && res.Job.VersionNumber != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "The install was cancelled. Version %d is on this machine; install it when you need it.\n", *res.Job.VersionNumber)
					return nil
				}
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				n := 0
				if res.Job.VersionNumber != nil {
					n = *res.Job.VersionNumber
				}
				switch {
				case res.Job.Status != "completed":
					fmt.Fprintf(out, "Pulling into %s (job %s: %s)\n", res.Job.ProjectName, res.Job.ID, res.Job.Status)
				case req.Install:
					fmt.Fprintf(out, "%s installed. Version %d is ready to use on this machine.\n", res.Job.ProjectName, n)
				default:
					fmt.Fprintf(out, "%s pulled. Version %d is on this machine. Install it when you need it.\n", res.Job.ProjectName, n)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&req.LocalName, "name", "", "Local name (default: the server name)")
	cmd.Flags().StringVar(&req.Path, "path", "", "Where the project folder is created")
	cmd.Flags().BoolVar(&req.Install, "install", false, "Install the latest version after pulling")
	return cmd
}

func newProjectPushCmd() *cobra.Command {
	var upTo int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "push <project>",
		Short: "Push local versions to the connected server",
		Long: `Push every version of a project that is not on the connected server yet,
oldest first. The first push creates the project on the server. Pushing
again when nothing changed does nothing.

Examples:
  nebi project push ml-baseline
  nebi project push ml-baseline --up-to 6`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if upTo < 0 {
				return fmt.Errorf("--up-to must be a version number")
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := resolveProjectID(ctx, s, args[0])
				if err != nil {
					return err
				}
				res, err := s.SDK.Remote.PushUpTo(ctx, id, upTo)
				if err != nil {
					return err
				}
				if asJSON {
					return writeJSON(res)
				}
				out := cmd.OutOrStdout()
				if len(res.PushedVersions) == 0 {
					fmt.Fprintln(out, "Nothing to push. This machine and the server are in sync.")
					return nil
				}
				fmt.Fprintf(out, "%s pushed (the server's newest version is %d).\n", capitalize(versionList(res.PushedVersions)), res.ServerLatestVersion)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&upTo, "up-to", 0, "Only push versions up to this number")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newProjectPullVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
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
				if asJSON {
					return writeJSON(v)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Version %d pulled. It is on this machine now. Install it to use it.\n", v.Number)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

// versionList is the prototype wording: "version 3" or "versions 3, 4 and 5".
func versionList(ns []int) string {
	if len(ns) == 0 {
		return "no versions"
	}
	if len(ns) == 1 {
		return fmt.Sprintf("version %d", ns[0])
	}
	parts := make([]string, len(ns)-1)
	for i, n := range ns[:len(ns)-1] {
		parts[i] = fmt.Sprint(n)
	}
	return fmt.Sprintf("versions %s and %d", strings.Join(parts, ", "), ns[len(ns)-1])
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// displayServerURL drops the scheme ("nebi.example.com").
func displayServerURL(u string) string {
	return strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://"), "/")
}

// syncedAgo is the "Last synced" wording of the Server page.
func syncedAgo(t *time.Time) string {
	if t == nil {
		return "never"
	}
	d := time.Since(*t)
	switch m := int(d.Minutes()); {
	case m < 1:
		return "just now"
	case m < 60:
		return fmt.Sprintf("%d minute%s ago", m, plural(m))
	default:
		h := m / 60
		if h < 48 {
			return fmt.Sprintf("%d hour%s ago", h, plural(h))
		}
		return t.Local().Format("2 Jan 2006, 15:04")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// versionSync is a tracked project's version sync with the connected server,
// computed by the client SDK (what the desktop app's project page shows).
type versionSync struct {
	InstalledVersion    *int   `json:"installed_version"`
	LatestVersion       *int   `json:"latest_version"`
	ServerURL           string `json:"server_url,omitempty"`
	ServerLatestVersion *int   `json:"server_latest_version"`
	UpdateAvailable     bool   `json:"update_available"`
	// NotPushed are local versions the server does not have yet.
	NotPushed []int `json:"not_pushed"`
	// ServerOnly are server versions this machine does not have yet.
	ServerOnly []int `json:"server_only"`
}

// projectVersionSync reads a project's version sync through the SDK. The
// server part stays empty when no server is connected, the project is not
// on it, or it cannot be reached.
func projectVersionSync(ctx context.Context, projectID uuid.UUID) (*versionSync, error) {
	stack, closeStack, err := client.OpenCLI(io.Discard)
	if err != nil {
		return nil, err
	}
	defer closeStack()
	item, err := stack.SDK.Local.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := &versionSync{
		InstalledVersion:    item.InstalledVersion,
		LatestVersion:       item.LatestVersion,
		ServerLatestVersion: item.ServerLatestVersion,
		UpdateAvailable:     item.UpdateAvailable,
		NotPushed:           []int{},
		ServerOnly:          []int{},
	}
	for _, r := range item.Remotes {
		if r.Kind == models.RemoteKindServer {
			out.ServerURL = r.URL
		}
	}
	if out.LatestVersion == nil && out.ServerURL == "" {
		// A folder tracked with 'nebi init' that has no versions yet.
		return nil, nil
	}
	if out.ServerLatestVersion == nil {
		return out, nil
	}
	versions, err := stack.SDK.Local.ListVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		switch {
		case v.ServerOnly:
			out.ServerOnly = append(out.ServerOnly, v.Number)
		case v.Number > *out.ServerLatestVersion:
			out.NotPushed = append(out.NotPushed, v.Number)
		}
	}
	slices.Sort(out.NotPushed)
	slices.Sort(out.ServerOnly)
	return out, nil
}

// printVersionSync writes the version part of `nebi status`.
func printVersionSync(w io.Writer, name string, v *versionSync) {
	installed := "not installed"
	if v.InstalledVersion != nil {
		installed = fmt.Sprintf("version %d installed", *v.InstalledVersion)
	}
	if v.LatestVersion != nil {
		fmt.Fprintf(w, "Versions:  newest here is %d, %s\n", *v.LatestVersion, installed)
	} else {
		fmt.Fprintf(w, "Versions:  none yet, %s\n", installed)
	}
	if v.ServerURL == "" {
		return
	}
	if v.ServerLatestVersion == nil {
		fmt.Fprintf(w, "Sync:      %s (server versions unavailable: not connected or not reachable)\n", displayServerURL(v.ServerURL))
		return
	}
	fmt.Fprintf(w, "Sync:      %s, newest on server is %d\n", displayServerURL(v.ServerURL), *v.ServerLatestVersion)
	if len(v.NotPushed) == 0 && len(v.ServerOnly) == 0 {
		fmt.Fprintln(w, "           This machine and the server are in sync.")
	}
	if len(v.NotPushed) > 0 {
		fmt.Fprintf(w, "           %s only on this machine. Push with 'nebi project push %s'.\n",
			capitalize(versionList(v.NotPushed))+isAre(v.NotPushed), name)
	}
	if len(v.ServerOnly) > 0 {
		fmt.Fprintf(w, "           Update available: %s only on the server. Pull with 'nebi project pull-version %s %d'.\n",
			versionList(v.ServerOnly)+isAre(v.ServerOnly), name, v.ServerOnly[len(v.ServerOnly)-1])
	}
}

func isAre(ns []int) string {
	if len(ns) == 1 {
		return " is"
	}
	return " are"
}
