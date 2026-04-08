package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var (
	wsVersionRemote   bool
	wsVersionJSON     bool
	wsVersionCreateMsg string
)

var workspaceVersionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"versions"},
	Short:   "View and manage workspace version history",
}

var workspaceVersionListCmd = &cobra.Command{
	Use:     "list [workspace]",
	Aliases: []string{"ls"},
	Short:   "List versions for a workspace",
	Long: `List version history for a workspace, newest first.

If no workspace name is given, the current directory's tracked workspace is used.

Examples:
  nebi workspace version list                  # current directory, local
  nebi workspace version list myws             # by name, local
  nebi workspace version list myws --remote    # by name, server`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkspaceVersionList,
}

var workspaceVersionShowCmd = &cobra.Command{
	Use:   "show <version> [workspace]",
	Short: "Show a single version's manifest, lock, and metadata",
	Long: `Show the contents of a specific workspace version.

If no workspace name is given, the current directory's tracked workspace is used.

Examples:
  nebi workspace version show 5                # current directory, local
  nebi workspace version show 5 myws           # by name, local
  nebi workspace version show 5 myws --remote  # by name, server`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runWorkspaceVersionShow,
}

var workspaceVersionCreateCmd = &cobra.Command{
	Use:   "create [workspace]",
	Short: "Create a version snapshot from disk (local only)",
	Long: `Create a new workspace version snapshot from pixi.toml and pixi.lock
in the workspace directory.

If no workspace name is given, the current directory's tracked workspace is used.
If the content is unchanged since the most recent snapshot, the existing
version is returned and no new record is created.

Server-side versions are created by 'nebi push', not by this command.

Examples:
  nebi workspace version create
  nebi workspace version create -m "Pinned numpy to 2.1"
  nebi workspace version create myws`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkspaceVersionCreate,
}

var workspaceVersionRollbackCmd = &cobra.Command{
	Use:   "rollback <version> [workspace]",
	Short: "Roll a workspace back to a previous version",
	Long: `Restore a workspace's pixi.toml and pixi.lock to a previous version
and create a new "Rolled back to version N" snapshot.

In local mode this writes the files to disk but does NOT run pixi install —
run it yourself afterwards to apply the change. In remote mode the rollback
is queued as a job on the server, which runs pixi install automatically.

Examples:
  nebi workspace version rollback 5                # current directory, local
  nebi workspace version rollback 5 myws           # by name, local
  nebi workspace version rollback 5 myws --remote  # by name, server`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runWorkspaceVersionRollback,
}

func init() {
	workspaceVersionListCmd.Flags().BoolVarP(&wsVersionRemote, "remote", "r", false, "Operate on the server instead of the local store")
	workspaceVersionListCmd.Flags().BoolVar(&wsVersionJSON, "json", false, "Output as JSON")

	workspaceVersionShowCmd.Flags().BoolVarP(&wsVersionRemote, "remote", "r", false, "Operate on the server instead of the local store")
	workspaceVersionShowCmd.Flags().BoolVar(&wsVersionJSON, "json", false, "Output as JSON")

	workspaceVersionCreateCmd.Flags().BoolVarP(&wsVersionRemote, "remote", "r", false, "(unsupported — use 'nebi push' to create versions on the server)")
	workspaceVersionCreateCmd.Flags().StringVarP(&wsVersionCreateMsg, "message", "m", "", "Description for the snapshot")

	workspaceVersionRollbackCmd.Flags().BoolVarP(&wsVersionRemote, "remote", "r", false, "Operate on the server instead of the local store")

	workspaceVersionCmd.AddCommand(workspaceVersionListCmd)
	workspaceVersionCmd.AddCommand(workspaceVersionShowCmd)
	workspaceVersionCmd.AddCommand(workspaceVersionCreateCmd)
	workspaceVersionCmd.AddCommand(workspaceVersionRollbackCmd)

	workspaceCmd.AddCommand(workspaceVersionCmd)
}

// resolveLocalWorkspace finds a tracked local workspace by name (if given)
// or by the current working directory.
func resolveLocalWorkspace(s *store.Store, name string) (*store.LocalWorkspace, error) {
	if name != "" {
		ws, err := s.FindWorkspaceByName(name)
		if err != nil {
			return nil, err
		}
		if ws == nil {
			return nil, fmt.Errorf("workspace %q not tracked locally; run 'nebi init' first", name)
		}
		return ws, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	ws, err := s.FindWorkspaceByPath(cwd)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		return nil, fmt.Errorf("no tracked workspace in %s; run 'nebi init' first", cwd)
	}
	return ws, nil
}

// resolveRemoteWorkspaceName picks the workspace name for a remote operation:
// the explicit positional, or the origin name from the current directory.
func resolveRemoteWorkspaceName(name string) (string, error) {
	if name != "" {
		return name, nil
	}
	origin, err := lookupOrigin()
	if err != nil {
		return "", err
	}
	if origin == nil {
		return "", fmt.Errorf("no workspace specified and no origin set in current directory")
	}
	return origin.OriginName, nil
}

func runWorkspaceVersionList(cmd *cobra.Command, args []string) error {
	var name string
	if len(args) == 1 {
		name = args[0]
	}

	if wsVersionRemote {
		return runWorkspaceVersionListRemote(name)
	}
	return runWorkspaceVersionListLocal(name)
}

func runWorkspaceVersionListLocal(name string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	ws, err := resolveLocalWorkspace(s, name)
	if err != nil {
		return err
	}

	versions, err := s.ListVersions(ws.ID)
	if err != nil {
		return err
	}

	if wsVersionJSON {
		return writeJSON(versions)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "No versions for workspace %q.\n", ws.Name)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tCREATED\tHASH\tDESCRIPTION")
	for _, v := range versions {
		hash := v.ContentHash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			v.VersionNumber,
			v.CreatedAt.Format("2006-01-02 15:04"),
			hash,
			v.Description,
		)
	}
	return w.Flush()
}

func runWorkspaceVersionListRemote(name string) error {
	wsName, err := resolveRemoteWorkspaceName(name)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		return err
	}

	versions, err := client.GetWorkspaceVersions(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("listing versions: %w", err)
	}

	if wsVersionJSON {
		return writeJSON(versions)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "No versions for workspace %q.\n", wsName)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tCREATED")
	for _, v := range versions {
		fmt.Fprintf(w, "%d\t%s\n", v.VersionNumber, formatTimestamp(v.CreatedAt))
	}
	return w.Flush()
}

func runWorkspaceVersionShow(cmd *cobra.Command, args []string) error {
	versionNum, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid version number %q: %w", args[0], err)
	}
	var name string
	if len(args) == 2 {
		name = args[1]
	}

	if wsVersionRemote {
		return runWorkspaceVersionShowRemote(name, versionNum)
	}
	return runWorkspaceVersionShowLocal(name, versionNum)
}

func runWorkspaceVersionShowLocal(name string, versionNum int) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	ws, err := resolveLocalWorkspace(s, name)
	if err != nil {
		return err
	}

	v, err := s.GetVersion(ws.ID, versionNum)
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("version %d not found for workspace %q", versionNum, ws.Name)
	}

	if wsVersionJSON {
		return writeJSON(v)
	}

	printVersion(ws.Name, v.VersionNumber, v.ContentHash, v.Description,
		v.CreatedAt.Format("2006-01-02 15:04"), v.ManifestContent, v.LockFileContent)
	return nil
}

func runWorkspaceVersionShowRemote(name string, versionNum int) error {
	wsName, err := resolveRemoteWorkspaceName(name)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		return err
	}

	manifest, err := client.GetVersionPixiToml(ctx, ws.ID, int32(versionNum))
	if err != nil {
		return fmt.Errorf("fetching pixi.toml: %w", err)
	}
	lock, err := client.GetVersionPixiLock(ctx, ws.ID, int32(versionNum))
	if err != nil {
		return fmt.Errorf("fetching pixi.lock: %w", err)
	}

	if wsVersionJSON {
		return writeJSON(map[string]any{
			"workspace":      wsName,
			"version_number": versionNum,
			"pixi_toml":      manifest,
			"pixi_lock":      lock,
		})
	}

	printVersion(wsName, versionNum, "", "", "", manifest, lock)
	return nil
}

// printVersion writes a human-readable rendering of a version to stdout.
// hash, description and createdAt may be empty (remote mode doesn't return them).
func printVersion(wsName string, versionNum int, hash, description, createdAt, manifest, lock string) {
	fmt.Printf("Workspace: %s\n", wsName)
	fmt.Printf("Version:   %d\n", versionNum)
	if createdAt != "" {
		fmt.Printf("Created:   %s\n", createdAt)
	}
	if hash != "" {
		fmt.Printf("Hash:      %s\n", hash)
	}
	if description != "" {
		fmt.Printf("Message:   %s\n", description)
	}
	fmt.Println()
	fmt.Println("--- pixi.toml ---")
	fmt.Println(manifest)
	if lock != "" {
		fmt.Println("--- pixi.lock ---")
		fmt.Println(lock)
	}
}

func runWorkspaceVersionCreate(cmd *cobra.Command, args []string) error {
	if wsVersionRemote {
		return fmt.Errorf("--remote not supported for create; use 'nebi push' to create versions on the server")
	}

	var name string
	if len(args) == 1 {
		name = args[0]
	}

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	ws, err := resolveLocalWorkspace(s, name)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(ws.Path, "pixi.toml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	// pixi.lock is optional
	lock, _ := os.ReadFile(filepath.Join(ws.Path, "pixi.lock"))

	description := wsVersionCreateMsg
	if description == "" {
		description = "Manual snapshot"
	}

	v, created, err := s.CreateVersion(ws.ID, string(manifest), string(lock), description)
	if err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}

	if !created {
		fmt.Fprintf(os.Stderr, "Content unchanged — reusing version %d (%s)\n",
			v.VersionNumber, v.ContentHash)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Created version %d (%s)\n", v.VersionNumber, v.ContentHash)
	return nil
}

func runWorkspaceVersionRollback(cmd *cobra.Command, args []string) error {
	versionNum, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid version number %q: %w", args[0], err)
	}
	var name string
	if len(args) == 2 {
		name = args[1]
	}

	if wsVersionRemote {
		return runWorkspaceVersionRollbackRemote(name, versionNum)
	}
	return runWorkspaceVersionRollbackLocal(name, versionNum)
}

func runWorkspaceVersionRollbackLocal(name string, versionNum int) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	ws, err := resolveLocalWorkspace(s, name)
	if err != nil {
		return err
	}

	v, err := s.RollbackToVersion(ws.ID, versionNum)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr,
		"Rolled back %s to version %d (now version %d). Run 'pixi install' to apply.\n",
		ws.Name, versionNum, v.VersionNumber,
	)
	return nil
}

func runWorkspaceVersionRollbackRemote(name string, versionNum int) error {
	wsName, err := resolveRemoteWorkspaceName(name)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		return err
	}

	job, err := client.RollbackWorkspace(ctx, ws.ID, versionNum)
	if err != nil {
		return fmt.Errorf("queuing rollback: %w", err)
	}

	fmt.Fprintf(os.Stderr,
		"Rollback queued for %s -> version %d (job %s)\n",
		wsName, versionNum, job.ID,
	)
	return nil
}

