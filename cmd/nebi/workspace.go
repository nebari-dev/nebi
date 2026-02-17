package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage tracked workspaces",
}

var wsListRemote bool

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List workspaces (local or on server)",
	Long: `List tracked workspaces. By default lists local workspaces.
With --remote, lists environments on the configured server.

Examples:
  nebi workspace list              # local workspaces
  nebi workspace list --remote     # workspaces on server`,
	Args: cobra.NoArgs,
	RunE: runWorkspaceList,
}

var workspaceTagsCmd = &cobra.Command{
	Use:   "tags <workspace-name>",
	Short: "List tags for a workspace on the server",
	Long: `List tags for a remote workspace.

Examples:
  nebi workspace tags myworkspace`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspaceTags,
}

var workspacePromoteCmd = &cobra.Command{
	Use:   "promote <name>",
	Short: "Copy current workspace to a global workspace",
	Long: `Create a global workspace by copying pixi.toml and pixi.lock
from the current tracked workspace directory.

The global workspace is stored in nebi's data directory and can be
referenced by name in commands like shell, run, and diff.

Examples:
  cd my-project
  nebi workspace promote data-science`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspacePromote,
}

var wsRemoveRemote bool

var workspaceRemoveCmd = &cobra.Command{
	Use:     "remove [name|path]",
	Aliases: []string{"rm"},
	Short:   "Remove a workspace from tracking",
	Long: `Remove a workspace from the local index or from the server.

By default removes from the local index:
  - With no argument or ".", removes the workspace tracked in the current directory.
  - For global workspaces, the stored files are also deleted.
  - For local workspaces, only the tracking entry is removed; project files are untouched.
  - A bare name refers to a global workspace; use a path (with a slash) for a local workspace.

With --remote, deletes the workspace from the configured server.

Examples:
  nebi workspace remove                     # remove workspace in current directory
  nebi workspace remove .                   # same as above
  nebi workspace remove data-science        # remove global workspace by name
  nebi workspace remove ./my-project        # remove local workspace by path
  nebi workspace remove myenv --remote      # delete workspace from server`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkspaceRemove,
}

var workspacePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove workspaces whose paths no longer exist",
	Long: `Remove all tracked workspaces whose directories are missing from disk.

For global workspaces with missing paths, the index entry is removed.
For local workspaces, the tracking entry is removed; no files are affected.

Examples:
  nebi workspace prune`,
	Args: cobra.NoArgs,
	RunE: runWorkspacePrune,
}

func init() {
	workspaceListCmd.Flags().BoolVar(&wsListRemote, "remote", false, "List workspaces on the server instead of locally")
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceTagsCmd)
	workspaceCmd.AddCommand(workspacePromoteCmd)
	workspaceRemoveCmd.Flags().BoolVar(&wsRemoveRemote, "remote", false, "Remove workspace from the server instead of locally")
	workspaceCmd.AddCommand(workspaceRemoveCmd)
	workspaceCmd.AddCommand(workspacePruneCmd)
}

func runWorkspaceList(cmd *cobra.Command, args []string) error {
	if wsListRemote {
		return runWorkspaceListServer()
	}
	return runWorkspaceListLocal()
}

func runWorkspaceListLocal() error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	wss, err := s.ListWorkspaces()
	if err != nil {
		return err
	}

	if len(wss) == 0 {
		fmt.Fprintln(os.Stderr, "No tracked workspaces. Run 'nebi init' in a pixi workspace to get started.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tPATH")
	var missing int
	for _, ws := range wss {
		wsType := "local"
		if s.IsGlobalWorkspace(&ws) {
			wsType = "global"
		}
		path := ws.Path
		if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
			path += " (missing)"
			missing++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", ws.Name, wsType, path)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if missing > 0 {
		fmt.Fprintf(os.Stderr, "\n%d workspace(s) have missing paths. Run 'nebi workspace prune' to clean up.\n", missing)
	}
	return nil
}

func runWorkspaceTags(cmd *cobra.Command, args []string) error {
	wsName := args[0]

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		return err
	}

	tags, err := client.GetWorkspaceTags(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("getting tags: %w", err)
	}

	if len(tags) == 0 {
		fmt.Fprintf(os.Stderr, "No tags for workspace %q.\n", wsName)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TAG\tVERSION\tCREATED\tUPDATED")
	for _, t := range tags {
		created := formatTimestamp(t.CreatedAt)
		updated := ""
		if t.UpdatedAt != t.CreatedAt {
			updated = formatTimestamp(t.UpdatedAt)
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", t.Tag, t.VersionNumber, created, updated)
	}
	return w.Flush()
}

func runWorkspaceListServer() error {
	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("listing workspaces: %w", err)
	}

	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "No workspaces on server.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tOWNER\tUPDATED")
	for _, ws := range workspaces {
		owner := "-"
		if ws.Owner != nil {
			owner = ws.Owner.Username
		}
		updated := ws.UpdatedAt.Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ws.Name, ws.Status, owner, updated)
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

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	// Verify current directory is a tracked workspace
	existing, err := s.FindWorkspaceByPath(cwd)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("current directory is not a tracked workspace; run 'nebi init' first")
	}

	// Check global name uniqueness
	existingGlobal, err := s.FindGlobalWorkspaceByName(name)
	if err != nil {
		return err
	}
	if existingGlobal != nil {
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
	wsDir := s.GlobalWorkspaceDir(name)

	if err := os.MkdirAll(wsDir, 0755); err != nil {
		return fmt.Errorf("creating global workspace directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(wsDir, "pixi.toml"), toml, 0644); err != nil {
		return fmt.Errorf("writing pixi.toml: %w", err)
	}

	if lock != nil {
		if err := os.WriteFile(filepath.Join(wsDir, "pixi.lock"), lock, 0644); err != nil {
			return fmt.Errorf("writing pixi.lock: %w", err)
		}
	}

	ws := &store.LocalWorkspace{
		Name: name,
		Path: wsDir,
	}
	if err := s.CreateWorkspace(ws); err != nil {
		return fmt.Errorf("saving workspace: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Global workspace %q created from %s\n", name, cwd)
	return nil
}

func runWorkspaceRemove(cmd *cobra.Command, args []string) error {
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}
	if wsRemoveRemote {
		if arg == "" || arg == "." {
			return fmt.Errorf("--remote requires a workspace name")
		}
		return runWorkspaceRemoveServer(arg)
	}
	return runWorkspaceRemoveLocal(arg)
}

func runWorkspaceRemoveServer(name string) error {
	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	ws, err := findWsByName(client, ctx, name)
	if err != nil {
		return err
	}

	if err := client.DeleteWorkspace(ctx, ws.ID); err != nil {
		return fmt.Errorf("deleting workspace: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Deleted workspace %q from server\n", name)
	return nil
}

func runWorkspaceRemoveLocal(arg string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	var ws *store.LocalWorkspace
	if arg == "" || arg == "." {
		// No argument or "." — remove workspace in current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		ws, err = s.FindWorkspaceByPath(cwd)
		if err != nil {
			return err
		}
		if ws == nil {
			return fmt.Errorf("no tracked workspace in current directory; run 'nebi workspace list' to see available workspaces")
		}
	} else if strings.Contains(arg, "/") || strings.Contains(arg, string(filepath.Separator)) {
		absPath, err := filepath.Abs(arg)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}
		ws, err = s.FindWorkspaceByPath(absPath)
		if err != nil {
			return err
		}
		if ws == nil {
			return fmt.Errorf("no tracked workspace at path %q", absPath)
		}
	} else {
		ws, err = s.FindWorkspaceByName(arg)
		if err != nil {
			return err
		}
		if ws == nil {
			return fmt.Errorf("workspace %q not found; use 'nebi workspace list' to see available workspaces", arg)
		}
	}

	isGlobal := s.IsGlobalWorkspace(ws)

	// Delete directory for global workspaces
	if isGlobal {
		if err := os.RemoveAll(ws.Path); err != nil {
			return fmt.Errorf("removing global workspace directory: %w", err)
		}
	}

	displayName := ws.Name
	if arg != "" && arg != "." {
		displayName = arg
	}

	if err := s.DeleteWorkspace(ws.ID); err != nil {
		return fmt.Errorf("removing workspace: %w", err)
	}

	if isGlobal {
		fmt.Fprintf(os.Stderr, "Removed global workspace %q\n", displayName)
	} else {
		fmt.Fprintf(os.Stderr, "Removed workspace %q (project files untouched)\n", displayName)
	}
	return nil
}

func runWorkspacePrune(cmd *cobra.Command, args []string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	wss, err := s.ListWorkspaces()
	if err != nil {
		return err
	}

	var pruned []string
	for _, ws := range wss {
		if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
			if err := s.DeleteWorkspace(ws.ID); err != nil {
				return fmt.Errorf("removing workspace %q: %w", ws.Name, err)
			}
			pruned = append(pruned, ws.Name)
		}
	}

	if len(pruned) == 0 {
		fmt.Fprintln(os.Stderr, "Nothing to prune.")
		return nil
	}

	for _, name := range pruned {
		fmt.Fprintf(os.Stderr, "Pruned %q\n", name)
	}
	fmt.Fprintf(os.Stderr, "Removed %d missing workspace(s).\n", len(pruned))
	return nil
}
