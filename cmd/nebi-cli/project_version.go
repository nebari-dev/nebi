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
	projectVersionRemote    bool
	projectVersionJSON      bool
	projectVersionCreateMsg string
)

var projectVersionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"versions"},
	Short:   "View and manage project version history",
}

var projectVersionListCmd = &cobra.Command{
	Use:     "list [project]",
	Aliases: []string{"ls"},
	Short:   "List versions for a project",
	Long: `List version history for a project, newest first.

If no project name is given, the current directory's tracked project is used.

Examples:
  nebi project version list                  # current directory, local
  nebi project version list myws             # by name, local
  nebi project version list myws --remote    # by name, server`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectVersionList,
}

var projectVersionShowCmd = &cobra.Command{
	Use:   "show <version> [project]",
	Short: "Show a single version's manifest, lock, and metadata",
	Long: `Show the contents of a specific project version.

If no project name is given, the current directory's tracked project is used.

Examples:
  nebi project version show 5                # current directory, local
  nebi project version show 5 myws           # by name, local
  nebi project version show 5 myws --remote  # by name, server`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runProjectVersionShow,
}

var projectVersionCreateCmd = &cobra.Command{
	Use:   "create [project]",
	Short: "Create a version snapshot from disk (local only)",
	Long: `Create a new project version snapshot from pixi.toml and pixi.lock
in the project directory.

If no project name is given, the current directory's tracked project is used.
If the content is unchanged since the most recent snapshot, the existing
version is returned and no new record is created.

Server-side versions are created by 'nebi push', not by this command.

Examples:
  nebi project version create
  nebi project version create -m "Pinned numpy to 2.1"
  nebi project version create myws`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectVersionCreate,
}

var projectVersionRollbackCmd = &cobra.Command{
	Use:   "rollback <version> [project]",
	Short: "Roll a project back to a previous version",
	Long: `Restore a project's pixi.toml and pixi.lock to a previous version
and create a new "Rolled back to snapshot N" snapshot.

In local mode this writes the files to disk but does NOT run pixi install —
run it yourself afterwards to apply the change. In remote mode the rollback
is queued as a job on the server, which runs pixi install automatically.

Examples:
  nebi project version rollback 5                # current directory, local
  nebi project version rollback 5 myws           # by name, local
  nebi project version rollback 5 myws --remote  # by name, server`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runProjectVersionRollback,
}

func init() {
	projectVersionListCmd.Flags().BoolVarP(&projectVersionRemote, "remote", "r", false, "Operate on the server instead of the local store")
	projectVersionListCmd.Flags().BoolVar(&projectVersionJSON, "json", false, "Output as JSON")

	projectVersionShowCmd.Flags().BoolVarP(&projectVersionRemote, "remote", "r", false, "Operate on the server instead of the local store")
	projectVersionShowCmd.Flags().BoolVar(&projectVersionJSON, "json", false, "Output as JSON")

	projectVersionCreateCmd.Flags().BoolVarP(&projectVersionRemote, "remote", "r", false, "(unsupported — use 'nebi push' to create versions on the server)")
	projectVersionCreateCmd.Flags().StringVarP(&projectVersionCreateMsg, "message", "m", "", "Description for the snapshot")

	projectVersionRollbackCmd.Flags().BoolVarP(&projectVersionRemote, "remote", "r", false, "Operate on the server instead of the local store")

	projectVersionCmd.AddCommand(projectVersionListCmd)
	projectVersionCmd.AddCommand(projectVersionShowCmd)
	projectVersionCmd.AddCommand(projectVersionCreateCmd)
	projectVersionCmd.AddCommand(projectVersionRollbackCmd)

	projectCmd.AddCommand(projectVersionCmd)
}

// resolveLocalProject finds a tracked local project by name (if given)
// or by the current working directory.
func resolveLocalProject(s *store.Store, name string) (*store.LocalProject, error) {
	if name != "" {
		project, err := s.FindProjectByName(name)
		if err != nil {
			return nil, err
		}
		if project == nil {
			return nil, fmt.Errorf("project %q not tracked locally; run 'nebi init' first", name)
		}
		return project, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	project, err := s.FindProjectByPath(cwd)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf("no tracked project in %s; run 'nebi init' first", cwd)
	}
	return project, nil
}

// resolveRemoteProjectName picks the project name for a remote operation:
// the explicit positional, or the origin name from the current directory.
func resolveRemoteProjectName(name string) (string, error) {
	if name != "" {
		return name, nil
	}
	origin, err := lookupOrigin()
	if err != nil {
		return "", err
	}
	if origin == nil {
		return "", fmt.Errorf("no project specified and no origin set in current directory")
	}
	return origin.OriginName, nil
}

func runProjectVersionList(cmd *cobra.Command, args []string) error {
	var name string
	if len(args) == 1 {
		name = args[0]
	}

	if projectVersionRemote {
		return runProjectVersionListRemote(name)
	}
	return runProjectVersionListLocal(name)
}

func runProjectVersionListLocal(name string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	project, err := resolveLocalProject(s, name)
	if err != nil {
		return err
	}

	versions, err := s.ListVersions(project.ID)
	if err != nil {
		return err
	}

	if projectVersionJSON {
		return writeJSON(versions)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "No versions for project %q.\n", project.Name)
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

func runProjectVersionListRemote(name string) error {
	projectName, err := resolveRemoteProjectName(name)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	project, err := findProjectByName(client, ctx, projectName)
	if err != nil {
		return err
	}

	versions, err := client.GetProjectVersions(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("listing versions: %w", err)
	}

	if projectVersionJSON {
		return writeJSON(versions)
	}

	if len(versions) == 0 {
		fmt.Fprintf(os.Stderr, "No versions for project %q.\n", projectName)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tCREATED")
	for _, v := range versions {
		fmt.Fprintf(w, "%d\t%s\n", v.VersionNumber, formatTimestamp(v.CreatedAt))
	}
	return w.Flush()
}

func runProjectVersionShow(cmd *cobra.Command, args []string) error {
	versionNum, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid version number %q: %w", args[0], err)
	}
	var name string
	if len(args) == 2 {
		name = args[1]
	}

	if projectVersionRemote {
		return runProjectVersionShowRemote(name, versionNum)
	}
	return runProjectVersionShowLocal(name, versionNum)
}

func runProjectVersionShowLocal(name string, versionNum int) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	project, err := resolveLocalProject(s, name)
	if err != nil {
		return err
	}

	v, err := s.GetVersion(project.ID, versionNum)
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("version %d not found for project %q", versionNum, project.Name)
	}

	if projectVersionJSON {
		return writeJSON(v)
	}

	printVersion(project.Name, v.VersionNumber, v.ContentHash, v.Description,
		v.CreatedAt.Format("2006-01-02 15:04"), v.ManifestContent, v.LockFileContent)
	return nil
}

func runProjectVersionShowRemote(name string, versionNum int) error {
	projectName, err := resolveRemoteProjectName(name)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	project, err := findProjectByName(client, ctx, projectName)
	if err != nil {
		return err
	}

	manifest, err := client.GetVersionPixiToml(ctx, project.ID, int32(versionNum))
	if err != nil {
		return fmt.Errorf("fetching pixi.toml: %w", err)
	}
	lock, err := client.GetVersionPixiLock(ctx, project.ID, int32(versionNum))
	if err != nil {
		return fmt.Errorf("fetching pixi.lock: %w", err)
	}

	if projectVersionJSON {
		return writeJSON(map[string]any{
			"project":        projectName,
			"version_number": versionNum,
			"pixi_toml":      manifest,
			"pixi_lock":      lock,
		})
	}

	printVersion(projectName, versionNum, "", "", "", manifest, lock)
	return nil
}

// printVersion writes a human-readable rendering of a version to stdout.
// hash, description and createdAt may be empty (remote mode doesn't return them).
func printVersion(projectName string, versionNum int, hash, description, createdAt, manifest, lock string) {
	fmt.Printf("Project: %s\n", projectName)
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

func runProjectVersionCreate(cmd *cobra.Command, args []string) error {
	if projectVersionRemote {
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

	project, err := resolveLocalProject(s, name)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(project.Path, "pixi.toml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	// pixi.lock is optional
	lock, _ := os.ReadFile(filepath.Join(project.Path, "pixi.lock"))

	description := projectVersionCreateMsg
	if description == "" {
		description = "Manual snapshot"
	}

	v, created, err := s.CreateVersion(project.ID, string(manifest), string(lock), description)
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

func runProjectVersionRollback(cmd *cobra.Command, args []string) error {
	versionNum, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid version number %q: %w", args[0], err)
	}
	var name string
	if len(args) == 2 {
		name = args[1]
	}

	if projectVersionRemote {
		return runProjectVersionRollbackRemote(name, versionNum)
	}
	return runProjectVersionRollbackLocal(name, versionNum)
}

func runProjectVersionRollbackLocal(name string, versionNum int) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	project, err := resolveLocalProject(s, name)
	if err != nil {
		return err
	}

	v, err := s.RollbackToVersion(project.ID, versionNum)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr,
		"Rolled back %s to version %d (now version %d). Run 'pixi install' to apply.\n",
		project.Name, versionNum, v.VersionNumber,
	)
	return nil
}

func runProjectVersionRollbackRemote(name string, versionNum int) error {
	projectName, err := resolveRemoteProjectName(name)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	project, err := findProjectByName(client, ctx, projectName)
	if err != nil {
		return err
	}

	job, err := client.RollbackProject(ctx, project.ID, versionNum)
	if err != nil {
		return fmt.Errorf("queuing rollback: %w", err)
	}

	fmt.Fprintf(os.Stderr,
		"Rollback queued for %s -> version %d (job %s)\n",
		projectName, versionNum, job.ID,
	)
	return nil
}
