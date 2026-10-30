package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage tracked projects",
}

var (
	projectListRemote    bool
	projectListJSON      bool
	projectListInstalled bool
	projectTagsJSON      bool
)

var projectListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List projects (local or on server)",
	Long: `List tracked projects. By default lists local projects.
With --remote, lists environments on the configured server.

Examples:
  nebi project list              # local projects
  nebi project list --remote     # projects on server`,
	Args: cobra.NoArgs,
	RunE: runProjectList,
}

var projectTagsCmd = &cobra.Command{
	Use:   "tags <project-name>",
	Short: "List tags for a project on the server",
	Long: `List tags for a remote project.

Examples:
  nebi project tags myproject`,
	Args:              cobra.ExactArgs(1),
	RunE:              runProjectTags,
	ValidArgsFunction: completeServerProjectNames,
}

var projectInstallCmd = &cobra.Command{
	Use:   "install <project-name>",
	Short: "Install a project's environment from its lockfile",
	Long: `Install a project's environment (.pixi/envs) from its lockfile.

The install runs as a server job; progress is streamed to the terminal.
Only available when the server runs in local mode.

Examples:
  nebi project install myproject`,
	Args:              cobra.ExactArgs(1),
	RunE:              runProjectInstall,
	ValidArgsFunction: completeServerProjectNames,
}

var projectUninstallCmd = &cobra.Command{
	Use:   "uninstall <project-name>",
	Short: "Remove a project's installed environment",
	Long: `Remove a project's installed environment (.pixi/envs).

The manifest (pixi.toml) and lockfile (pixi.lock) are kept, so the
project can be reinstalled later. Only available when the server runs
in local mode.

Examples:
  nebi project uninstall myproject`,
	Args:              cobra.ExactArgs(1),
	RunE:              runProjectUninstall,
	ValidArgsFunction: completeServerProjectNames,
}

var projectRemoveRemote bool

var projectRemoveCmd = &cobra.Command{
	Use:     "remove [name|path]",
	Aliases: []string{"rm"},
	Short:   "Remove a project from tracking",
	Long: `Remove a project from the local index or from the server.

By default removes from the local index:
  - With no argument or ".", removes the project tracked in the current directory.
  - Only the tracking entry is removed; project files are untouched.
  - A bare name looks up a project by name; use a path (with a slash) for a path-based lookup.

With --remote, deletes the project from the configured server.

Examples:
  nebi project remove                     # remove project in current directory
  nebi project remove .                   # same as above
  nebi project remove data-science        # remove project by name
  nebi project remove ./my-project        # remove project by path
  nebi project remove myenv --remote      # delete project from server`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runProjectRemove,
	ValidArgsFunction: completeProjectRemove,
}

// projectDirMissing returns true if neither pixi.toml nor pixi.lock exist under the given path.
func projectDirMissing(path string) bool {
	_, tomlErr := os.Stat(filepath.Join(path, "pixi.toml"))
	_, lockErr := os.Stat(filepath.Join(path, "pixi.lock"))
	return os.IsNotExist(tomlErr) && os.IsNotExist(lockErr)
}

var projectPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove projects whose paths no longer exist",
	Long: `Remove all tracked projects whose directories are missing from disk.

The tracking entry is removed; no files are affected.

Examples:
  nebi project prune`,
	Args: cobra.NoArgs,
	RunE: runProjectPrune,
}

func init() {
	projectListCmd.Flags().BoolVarP(&projectListRemote, "remote", "r", false, "List projects on the server instead of locally")
	projectListCmd.Flags().BoolVar(&projectListJSON, "json", false, "Output as JSON")
	projectListCmd.Flags().BoolVar(&projectListInstalled, "installed", false, "Only list server projects with an installed environment")
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectInstallCmd)
	projectCmd.AddCommand(projectUninstallCmd)
	projectTagsCmd.Flags().BoolVar(&projectTagsJSON, "json", false, "Output as JSON")
	projectCmd.AddCommand(projectTagsCmd)
	projectRemoveCmd.Flags().BoolVarP(&projectRemoveRemote, "remote", "r", false, "Remove project from the server instead of locally")
	projectCmd.AddCommand(projectRemoveCmd)
	projectCmd.AddCommand(projectPruneCmd)
}

func runProjectList(cmd *cobra.Command, args []string) error {
	if projectListRemote || projectListInstalled {
		return runProjectListServer()
	}
	return runProjectListLocal()
}

func runProjectInstall(cmd *cobra.Command, args []string) error {
	return runProjectEnvJob(args[0], "install")
}

func runProjectUninstall(cmd *cobra.Command, args []string) error {
	return runProjectEnvJob(args[0], "uninstall")
}

// runProjectEnvJob enqueues an install or uninstall job for the named
// project and streams the job's logs until it finishes.
func runProjectEnvJob(projectName, action string) error {
	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	project, err := findProjectByName(client, ctx, projectName)
	if err != nil {
		return err
	}

	var job *cliclient.Job
	if action == "install" {
		job, err = client.InstallProject(ctx, project.ID)
	} else {
		job, err = client.UninstallProject(ctx, project.ID)
	}
	if err != nil {
		return fmt.Errorf("starting %s: %w", action, err)
	}

	fmt.Fprintf(os.Stderr, "Started %s job %s for project %q\n", action, job.ID, projectName)

	if err := client.StreamJobLogs(ctx, job.ID, os.Stdout); err != nil {
		return fmt.Errorf("streaming %s logs: %w", action, err)
	}

	// The stream ends on the done event; the job's final status decides
	// the exit code.
	final, err := client.GetJob(ctx, job.ID)
	if err != nil {
		return fmt.Errorf("checking %s result: %w", action, err)
	}
	if final.Status == "failed" {
		if final.Error != "" {
			return fmt.Errorf("%s failed: %s", action, final.Error)
		}
		return fmt.Errorf("%s failed", action)
	}

	fmt.Fprintf(os.Stderr, "Project %q %sed successfully\n", projectName, action)
	return nil
}

func runProjectListLocal() error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	projects, err := s.ListProjects()
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		if projectListJSON {
			return writeJSON([]store.LocalProject{})
		}
		fmt.Fprintln(os.Stderr, "No tracked projects. Run 'nebi init' in a pixi workspace to get started.")
		return nil
	}

	// Sync project names from pixi.toml before displaying
	for i := range projects {
		if err := syncProjectName(s, &projects[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", projects[i].Path, err)
		}
	}

	if projectListJSON {
		type item struct {
			store.LocalProject
			Missing bool `json:"missing"`
		}
		items := make([]item, len(projects))
		for i, project := range projects {
			items[i] = item{
				LocalProject: project,
				Missing:      projectDirMissing(project.Path),
			}
		}
		return writeJSON(items)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tORIGIN\tORIGIN_ID\tID\tPATH")
	var missing int
	for _, project := range projects {
		path := project.Path
		if projectDirMissing(project.Path) {
			path += " (missing)"
			missing++
		}
		origin := "-"
		if project.OriginName != "" {
			origin = project.OriginName
		}
		originID := "-"
		if project.OriginID != "" {
			originID = project.OriginID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", project.Name, origin, originID, project.ID.String(), path)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if missing > 0 {
		fmt.Fprintf(os.Stderr, "\n%d project(s) have missing paths. Run 'nebi project prune' to clean up.\n", missing)
	}
	return nil
}

func runProjectTags(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	project, err := findProjectByName(client, ctx, projectName)
	if err != nil {
		return err
	}

	tags, err := client.GetProjectTags(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("getting tags: %w", err)
	}

	if len(tags) == 0 {
		if projectTagsJSON {
			return writeJSON([]cliclient.ProjectTag{})
		}
		fmt.Fprintf(os.Stderr, "No tags for project %q.\n", projectName)
		return nil
	}

	if projectTagsJSON {
		return writeJSON(tags)
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

func runProjectListServer() error {
	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}

	if projectListInstalled {
		installed := projects[:0]
		for _, project := range projects {
			if project.InstallStatus == "installed" {
				installed = append(installed, project)
			}
		}
		projects = installed
	}

	if len(projects) == 0 {
		if projectListJSON {
			return writeJSON([]cliclient.Project{})
		}
		if projectListInstalled {
			fmt.Fprintln(os.Stderr, "No installed projects on server.")
		} else {
			fmt.Fprintln(os.Stderr, "No projects on server.")
		}
		return nil
	}

	if projectListJSON {
		return writeJSON(projects)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tINSTALL\tOWNER\tUPDATED")
	for _, project := range projects {
		owner := "-"
		if project.Owner != nil {
			owner = project.Owner.Username
		}
		install := project.InstallStatus
		if install == "" {
			install = "-"
		}
		updated := project.UpdatedAt.Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", project.Name, project.Status, install, owner, updated)
	}
	return w.Flush()
}

func runProjectRemove(cmd *cobra.Command, args []string) error {
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}
	if projectRemoveRemote {
		if arg == "" || arg == "." {
			return fmt.Errorf("--remote requires a project name")
		}
		return runProjectRemoveServer(arg)
	}
	return runProjectRemoveLocal(arg)
}

func runProjectRemoveServer(name string) error {
	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	project, err := findProjectByName(client, ctx, name)
	if err != nil {
		return err
	}

	if err := client.DeleteProject(ctx, project.ID); err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Deleted project %q from server\n", name)
	return nil
}

func runProjectRemoveLocal(arg string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	var project *store.LocalProject
	if arg == "" || arg == "." {
		// No argument or "." — remove project in current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		project, err = s.FindProjectByPath(cwd)
		if err != nil {
			return err
		}
		if project == nil {
			return fmt.Errorf("no tracked project in current directory; run 'nebi project list' to see available projects")
		}
	} else if strings.Contains(arg, "/") || strings.Contains(arg, string(filepath.Separator)) {
		absPath, err := filepath.Abs(arg)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}
		project, err = s.FindProjectByPath(absPath)
		if err != nil {
			return err
		}
		if project == nil {
			return fmt.Errorf("no tracked project at path %q", absPath)
		}
	} else {
		projects, err := findProjectsByNameWithSync(s, arg)
		if err != nil {
			return err
		}
		switch len(projects) {
		case 0:
			return fmt.Errorf("project %q not found; use 'nebi project list' to see available projects", arg)
		case 1:
			project = &projects[0]
		default:
			project, err = pickProject(projects, arg)
			if err != nil {
				return err
			}
		}
	}

	displayName := project.Name
	if arg != "" && arg != "." {
		displayName = arg
	}

	if err := s.DeleteProject(project.ID); err != nil {
		return fmt.Errorf("removing project: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Removed project %q (project files untouched)\n", displayName)
	return nil
}

func runProjectPrune(cmd *cobra.Command, args []string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	projects, err := s.ListProjects()
	if err != nil {
		return err
	}

	// Sync project names from pixi.toml before pruning
	for i := range projects {
		if err := syncProjectName(s, &projects[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", projects[i].Path, err)
		}
	}

	var pruned []string
	for _, project := range projects {
		if projectDirMissing(project.Path) {
			if err := s.DeleteProject(project.ID); err != nil {
				return fmt.Errorf("removing project %q: %w", project.Name, err)
			}
			pruned = append(pruned, project.Name)
		}
	}

	if len(pruned) == 0 {
		fmt.Fprintln(os.Stderr, "Nothing to prune.")
		return nil
	}

	for _, name := range pruned {
		fmt.Fprintf(os.Stderr, "Pruned %q\n", name)
	}
	fmt.Fprintf(os.Stderr, "Removed %d missing project(s).\n", len(pruned))
	return nil
}
