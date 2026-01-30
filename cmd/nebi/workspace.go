package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage tracked workspaces",
}

var wsListServer string

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List workspaces (local or on a server)",
	Long: `List tracked workspaces. Without -s, lists local workspaces.
With -s, lists environments on the specified server.

Examples:
  nebi workspace list              # local workspaces
  nebi workspace list -s work      # workspaces on server "work"`,
	Args: cobra.NoArgs,
	RunE: runWorkspaceList,
}

var wsTagsServer string

var workspaceTagsCmd = &cobra.Command{
	Use:   "tags <workspace-name>",
	Short: "List tags for a workspace on a server",
	Long: `List tags for a remote workspace.

Examples:
  nebi workspace tags myworkspace -s work`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspaceTags,
}

func init() {
	workspaceListCmd.Flags().StringVarP(&wsListServer, "server", "s", "", "List workspaces on a server instead of locally")
	workspaceTagsCmd.Flags().StringVarP(&wsTagsServer, "server", "s", "", "Server name or URL (uses default if not set)")
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceTagsCmd)
}

func runWorkspaceList(cmd *cobra.Command, args []string) error {
	if wsListServer != "" {
		return runWorkspaceListServer()
	}
	return runWorkspaceListLocal()
}

func runWorkspaceListLocal() error {
	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	if len(idx.Workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "No tracked workspaces. Run 'nebi init' in a pixi workspace to get started.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATH")
	for _, ws := range idx.Workspaces {
		fmt.Fprintf(w, "%s\t%s\n", ws.Name, ws.Path)
	}
	return w.Flush()
}

func runWorkspaceTags(cmd *cobra.Command, args []string) error {
	wsName := args[0]

	server, err := resolveServerFlag(wsTagsServer)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient(server)
	if err != nil {
		return err
	}

	ctx := context.Background()

	env, err := findEnvByName(client, ctx, wsName)
	if err != nil {
		return err
	}

	tags, err := client.GetEnvironmentTags(ctx, env.ID)
	if err != nil {
		return fmt.Errorf("getting tags: %w", err)
	}

	if len(tags) == 0 {
		fmt.Fprintf(os.Stderr, "No tags for workspace %q.\n", wsName)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TAG\tVERSION")
	for _, t := range tags {
		fmt.Fprintf(w, "%s\t%d\n", t.Tag, t.VersionNumber)
	}
	return w.Flush()
}

func runWorkspaceListServer() error {
	client, err := getAuthenticatedClient(wsListServer)
	if err != nil {
		return err
	}

	ctx := context.Background()
	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("listing environments: %w", err)
	}

	if len(envs) == 0 {
		fmt.Fprintln(os.Stderr, "No workspaces on server.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS")
	for _, env := range envs {
		fmt.Fprintf(w, "%s\t%s\n", env.Name, env.Status)
	}
	return w.Flush()
}
