package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
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

var workspacePromoteCmd = &cobra.Command{
	Use:   "promote <name>",
	Short: "Copy current workspace to a global workspace",
	Long: `Create a global workspace by copying pixi.toml and pixi.lock
from the current tracked workspace directory.

The global workspace is stored in nebi's data directory and can be
referenced by name in commands like diff and shell.

Examples:
  cd my-project
  nebi workspace promote data-science`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspacePromote,
}

var workspaceRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a workspace from tracking",
	Long: `Remove a workspace from the local index.

For global workspaces, the stored files are also deleted.
For local workspaces, only the tracking entry is removed; project files are untouched.

A bare name refers to a global workspace; use a path (with a slash) for a local workspace.

Examples:
  nebi workspace remove data-science   # remove global workspace by name
  nebi workspace remove ./my-project   # remove local workspace by path`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspaceRemove,
}

func init() {
	workspaceListCmd.Flags().StringVarP(&wsListServer, "server", "s", "", "List workspaces on a server instead of locally")
	workspaceTagsCmd.Flags().StringVarP(&wsTagsServer, "server", "s", "", "Server name or URL (uses default if not set)")
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceTagsCmd)
	workspaceCmd.AddCommand(workspacePromoteCmd)
	workspaceCmd.AddCommand(workspaceRemoveCmd)
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
	fmt.Fprintln(w, "NAME\tTYPE\tPATH")
	for _, ws := range idx.Workspaces {
		wsType := "local"
		if ws.Global {
			wsType = "global"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", ws.Name, wsType, ws.Path)
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
	fmt.Fprintln(w, "NAME\tSTATUS\tOWNER\tUPDATED")
	for _, env := range envs {
		owner := "-"
		if env.Owner != nil {
			owner = env.Owner.Username
		}
		updated := env.UpdatedAt.Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", env.Name, env.Status, owner, updated)
	}
	return w.Flush()
}

func runWorkspacePromote(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := validateWorkspaceName(name); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	// Verify current directory is a tracked workspace
	if _, exists := idx.Workspaces[cwd]; !exists {
		return fmt.Errorf("current directory is not a tracked workspace; run 'nebi init' first")
	}

	// Check global name uniqueness
	if existing := findGlobalWorkspaceByName(idx, name); existing != nil {
		return fmt.Errorf("a global workspace named %q already exists", name)
	}

	// Read source files
	toml, err := os.ReadFile(filepath.Join(cwd, "pixi.toml"))
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}

	var lock []byte
	lockData, err := os.ReadFile(filepath.Join(cwd, "pixi.lock"))
	if err == nil {
		lock = lockData
	}

	// Create global workspace
	id := uuid.New().String()
	envDir := store.GlobalEnvDir(id)

	if err := os.MkdirAll(envDir, 0755); err != nil {
		return fmt.Errorf("creating global workspace directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(envDir, "pixi.toml"), toml, 0644); err != nil {
		return fmt.Errorf("writing pixi.toml: %w", err)
	}

	if lock != nil {
		if err := os.WriteFile(filepath.Join(envDir, "pixi.lock"), lock, 0644); err != nil {
			return fmt.Errorf("writing pixi.lock: %w", err)
		}
	}

	idx.Workspaces[envDir] = &localstore.Workspace{
		ID:     id,
		Name:   name,
		Path:   envDir,
		Global: true,
	}

	if err := store.SaveIndex(idx); err != nil {
		return fmt.Errorf("saving index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Global workspace %q created from %s\n", name, cwd)
	return nil
}

func runWorkspaceRemove(cmd *cobra.Command, args []string) error {
	arg := args[0]

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	var ws *localstore.Workspace
	if strings.Contains(arg, "/") || strings.Contains(arg, string(filepath.Separator)) {
		// Argument contains a slash — treat as a path
		absPath, err := filepath.Abs(arg)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}
		if w, exists := idx.Workspaces[absPath]; exists {
			ws = w
		}
		if ws == nil {
			return fmt.Errorf("no tracked workspace at path %q", absPath)
		}
	} else {
		// No slash — treat as a global workspace name
		ws = findGlobalWorkspaceByName(idx, arg)
		if ws == nil {
			return fmt.Errorf("global workspace %q not found; use 'nebi workspace list' to see available workspaces\nTo remove a local workspace, use a path (e.g. ./myproject)", arg)
		}
	}

	// Remove from index
	delete(idx.Workspaces, ws.Path)

	// Delete directory for global workspaces
	if ws.Global {
		if err := os.RemoveAll(ws.Path); err != nil {
			return fmt.Errorf("removing global workspace directory: %w", err)
		}
	}

	if err := store.SaveIndex(idx); err != nil {
		return fmt.Errorf("saving index: %w", err)
	}

	if ws.Global {
		fmt.Fprintf(os.Stderr, "Removed global workspace %q\n", arg)
	} else {
		fmt.Fprintf(os.Stderr, "Removed workspace %q (project files untouched)\n", arg)
	}
	return nil
}
