package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pixi"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
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

var projectInstallVersion int

var projectInstallCmd = &cobra.Command{
	Use:   "install <project-name>",
	Short: "Install a version of a project on this machine",
	Long: `Install a project's environment (.pixi/envs) from a version's lockfile.

One version is installed at a time: installing a version replaces the one
that is installed. Without --version, the last installed version is
installed again, else the latest version. The log is streamed to the
terminal; Ctrl-C cancels the install.

A project that is not on this machine is installed through the configured
server instead (local-mode servers only).

Examples:
  nebi project install myproject
  nebi project install myproject --version 3`,
	Args:              cobra.ExactArgs(1),
	RunE:              runProjectInstall,
	ValidArgsFunction: completeServerProjectNames,
}

var projectUninstallCmd = &cobra.Command{
	Use:   "uninstall <project-name>",
	Short: "Remove a project's installed environment",
	Long: `Remove a project's installed environment (.pixi/envs).

The project and all its versions stay, and Nebi remembers the version
so it can be installed again. A project that is not on this machine is
uninstalled through the configured server instead (local-mode servers
only).

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
	projectInstallCmd.Flags().IntVar(&projectInstallVersion, "version", 0, "Version number to install (default: last installed, else latest)")
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
	return runProjectListLocal(cmd)
}

// runProjectInstall installs a project on this machine through the client
// SDK (the version model: one installed version at a time). Projects that
// are not on this machine fall back to the server's install job (legacy
// behaviour, local-mode servers only).
func runProjectInstall(cmd *cobra.Command, args []string) error {
	var version *int
	if projectInstallVersion > 0 {
		v := projectInstallVersion
		version = &v
	}
	handled, err := runLocalEnvJob(cmd, args[0], func(ctx context.Context, s *client.Stack, id uuid.UUID) (*shared.JobSummary, error) {
		return s.SDK.Exec.InstallVersion(ctx, id, shared.InstallRequest{Version: version})
	})
	if handled {
		return err
	}
	if version != nil {
		return fmt.Errorf("project %q is not on this machine; --version only works for local projects", args[0])
	}
	return runProjectEnvJob(args[0], "install")
}

// runProjectUninstall removes the installed environment of a local project
// through the client SDK, falling back to the server job for projects that
// are not on this machine.
func runProjectUninstall(cmd *cobra.Command, args []string) error {
	handled, err := runLocalEnvJob(cmd, args[0], func(ctx context.Context, s *client.Stack, id uuid.UUID) (*shared.JobSummary, error) {
		return s.SDK.Exec.Uninstall(ctx, id)
	})
	if handled {
		return err
	}
	return runProjectEnvJob(args[0], "uninstall")
}

// runLocalEnvJob runs fn for a project of the local database. handled is
// false when no local project matches ref, so the caller can fall back to
// the server.
func runLocalEnvJob(cmd *cobra.Command, ref string, fn func(context.Context, *client.Stack, uuid.UUID) (*shared.JobSummary, error)) (handled bool, err error) {
	err = withStack(cmd, func(ctx context.Context, s *client.Stack) error {
		p, err := s.SDK.Local.ResolveProject(ctx, ref)
		if errors.Is(err, shared.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		handled = true
		job, err := fn(ctx, s, p.ID)
		if job != nil {
			version := ""
			if job.VersionNumber != nil {
				version = fmt.Sprintf(" version %d", *job.VersionNumber)
			}
			fmt.Fprintf(os.Stderr, "%s%s of %s: %s\n", job.TypeLabel, version, p.Name, projectJobStatusLabel(job.Status))
		}
		return err
	})
	return handled, err
}

// projectJobStatusLabel is the UI label of a job status (Queued, Running, ...).
func projectJobStatusLabel(status models.JobStatus) string {
	switch status {
	case models.JobStatusPending:
		return "Queued"
	case models.JobStatusRunning:
		return "Running"
	case models.JobStatusCompleted:
		return "Succeeded"
	case models.JobStatusFailed:
		return "Failed"
	case models.JobStatusCancelled:
		return "Cancelled"
	}
	return string(status)
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

// syncTrackedProjectNames renames workspaces tracked with 'nebi init' after
// their pixi.toml (the legacy list behaviour) and returns each tracked
// workspace's origin label. Projects created through Nebi (they have an
// owner) keep their name: their pixi.toml workspace name may differ.
func syncTrackedProjectNames(ctx context.Context, db *gorm.DB) (map[uuid.UUID]string, error) {
	var tracked []store.LocalProject
	if err := db.WithContext(ctx).Where("owner_id IS NULL OR owner_id = ''").Find(&tracked).Error; err != nil {
		return nil, err
	}
	origins := map[uuid.UUID]string{}
	for i := range tracked {
		p := &tracked[i]
		if content, err := os.ReadFile(filepath.Join(p.Path, "pixi.toml")); err == nil {
			tomlName, err := pixi.ExtractWorkspaceName(string(content))
			switch {
			case err != nil:
				fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", p.Path, err)
			case tomlName != p.Name:
				if err := db.WithContext(ctx).Model(&store.LocalProject{}).Where("id = ?", p.ID).Update("name", tomlName).Error; err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %s: updating project name: %v\n", p.Path, err)
				} else {
					fmt.Fprintf(os.Stderr, "Project name updated: %q -> %q (from pixi.toml)\n", p.Name, tomlName)
				}
			}
		}
		if p.OriginName != "" {
			origin := p.OriginName
			if p.OriginTag != "" {
				origin += ":" + p.OriginTag
			}
			origins[p.ID] = origin
		}
	}
	return origins, nil
}

// projectListRow is one row of 'nebi project list --json': the client API's
// project list item plus whether its folder is missing.
type projectListRow struct {
	shared.ProjectListItem
	Missing bool `json:"missing"`
}

// runProjectListLocal lists the projects on this machine through the client
// SDK: the same list as the Projects page (name, installed version,
// environment, size, connected remotes, path), including workspaces tracked
// with 'nebi init'.
func runProjectListLocal(cmd *cobra.Command) error {
	return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
		origins, err := syncTrackedProjectNames(ctx, s.SDK.Deps().DB)
		if err != nil {
			return err
		}
		items, err := s.SDK.Local.ListProjects(ctx)
		if err != nil {
			return err
		}
		rows := make([]projectListRow, len(items))
		missing := 0
		for i, it := range items {
			rows[i] = projectListRow{ProjectListItem: it, Missing: projectDirMissing(it.Path)}
			if rows[i].Missing {
				missing++
			}
		}
		if projectListJSON {
			return writeJSON(rows)
		}
		if len(rows) == 0 {
			fmt.Fprintln(os.Stderr, "No projects yet. Create one with 'nebi project create', or run 'nebi init' in a pixi workspace.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tENVIRONMENT\tSIZE\tREMOTES\tPATH")
		for _, r := range rows {
			path := r.Path
			if r.Missing {
				path += " (missing)"
			}
			remotes := projectRemoteNames(r.ProjectListItem)
			if remotes == "-" && origins[r.ID] != "" {
				remotes = origins[r.ID]
			}
			size := r.Size
			if size == "" {
				size = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, projectListVersion(r.ProjectListItem),
				projectEnvironmentLabel(r.ProjectListItem), size, remotes, path)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if missing > 0 {
			fmt.Fprintf(os.Stderr, "\n%d project(s) have missing paths. Run 'nebi project prune' to clean up.\n", missing)
		}
		return nil
	})
}

// projectListVersion is the Version column: the installed version, else the last
// installed one, else the latest, with "update available" when the linked
// server has newer versions.
func projectListVersion(p shared.ProjectListItem) string {
	v := "-"
	switch {
	case p.InstalledVersion != nil:
		v = fmt.Sprintf("v%d", *p.InstalledVersion)
	case p.LastInstalledVersion != nil:
		v = fmt.Sprintf("v%d (last installed)", *p.LastInstalledVersion)
	case p.LatestVersion != nil:
		v = fmt.Sprintf("v%d", *p.LatestVersion)
	}
	if p.UpdateAvailable {
		v += ", update available"
	}
	return v
}

// projectEnvironmentLabel is the Environment column.
func projectEnvironmentLabel(p shared.ProjectListItem) string {
	switch p.Status {
	case models.ProjectStatusPending, models.ProjectStatusCreating:
		return "creating"
	case models.ProjectStatusDeleting:
		return "deleting"
	case models.ProjectStatusFailed:
		if p.LatestVersion == nil {
			return "create failed"
		}
	}
	switch p.InstallStatus {
	case models.InstallStatusInstalled:
		return "installed"
	case models.InstallStatusInstalling:
		return "installing"
	case models.InstallStatusUninstalling:
		return "uninstalling"
	case models.InstallStatusFailed:
		if p.FailedInstallJobID != nil {
			return "install failed (job " + p.FailedInstallJobID.String() + ")"
		}
		return "install failed"
	}
	return "not installed"
}

// projectRemoteNames joins the connected remotes' names.
func projectRemoteNames(p shared.ProjectListItem) string {
	if len(p.Remotes) == 0 {
		return "-"
	}
	names := make([]string, len(p.Remotes))
	for i, r := range p.Remotes {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
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
