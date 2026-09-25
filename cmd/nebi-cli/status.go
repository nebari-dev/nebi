package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var statusJSON bool

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
}

type statusResult struct {
	Project      string `json:"project"`
	Path         string `json:"path"`
	Server       string `json:"server,omitempty"`
	OriginName   string `json:"origin_name,omitempty"`
	OriginTag    string `json:"origin_tag,omitempty"`
	OriginAction string `json:"origin_action,omitempty"`
	TomlModified bool   `json:"toml_modified"`
	LockModified bool   `json:"lock_modified"`
	ServerSync   string `json:"server_sync,omitempty"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project sync status",
	Long: `Show the current project's tracking info and sync status with the server.

Displays the project name, path, and origin info for the
last push/pull operation.

If the server is reachable, checks whether the local files or server version
have changed since the last sync.

Examples:
  nebi status`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	project, err := s.FindProjectByPath(cwd)
	if err != nil {
		return err
	}
	if project == nil {
		if statusJSON {
			return fmt.Errorf("not a tracked project")
		}
		fmt.Fprintln(os.Stderr, "Not a tracked project. Run 'nebi init'.")
		return nil
	}

	// Sync project name if pixi.toml has changed
	if err := syncProjectName(s, project); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	serverURL, _ := s.LoadServerURL()

	if statusJSON {
		return runStatusJSON(s, project, serverURL, cwd)
	}

	fmt.Fprintf(os.Stdout, "Project: %s\n", project.Name)
	fmt.Fprintf(os.Stdout, "Path:      %s\n", project.Path)

	if serverURL != "" {
		fmt.Fprintf(os.Stdout, "Server:    %s\n", serverURL)
	} else {
		fmt.Fprintln(os.Stdout, "Server:    (not configured)")
	}

	if project.OriginName == "" {
		fmt.Fprintln(os.Stdout, "\nNo origin. Push or pull to set an origin.")
		return nil
	}

	// Check local file modifications against stored hashes
	localToml, _ := os.ReadFile(filepath.Join(cwd, "pixi.toml"))
	localLock, _ := os.ReadFile(filepath.Join(cwd, "pixi.lock"))
	localTomlHash, err := store.TomlContentHash(string(localToml))
	if err != nil {
		return fmt.Errorf("hashing local pixi.toml: %w", err)
	}
	localLockHash := store.ContentHash(string(localLock))

	fmt.Fprintln(os.Stdout)

	if project.OriginTomlHash != "" && project.OriginTomlHash != localTomlHash {
		fmt.Fprintln(os.Stdout, "pixi.toml modified locally")
	}
	if project.OriginLockHash != "" && project.OriginLockHash != localLockHash {
		fmt.Fprintln(os.Stdout, "pixi.lock modified locally")
	}

	fmt.Fprintln(os.Stdout, "\nOrigin:")
	fmt.Fprintf(os.Stdout, "  %s:%s (%s)\n", project.OriginName, project.OriginTag, project.OriginAction)

	if serverURL != "" {
		serverStatus := checkServerOrigin(s, serverURL, project)
		if serverStatus != "" {
			fmt.Fprintf(os.Stdout, "  %s\n", serverStatus)
		}
	}

	return nil
}

func runStatusJSON(s *store.Store, project *store.LocalProject, serverURL, cwd string) error {
	result := statusResult{
		Project:      project.Name,
		Path:         project.Path,
		Server:       serverURL,
		OriginName:   project.OriginName,
		OriginTag:    project.OriginTag,
		OriginAction: project.OriginAction,
	}

	if project.OriginName == "" {
		return writeJSON(result)
	}

	// Check local file modifications against stored hashes
	localToml, _ := os.ReadFile(filepath.Join(cwd, "pixi.toml"))
	localLock, _ := os.ReadFile(filepath.Join(cwd, "pixi.lock"))
	localTomlHash, err := store.TomlContentHash(string(localToml))
	if err != nil {
		return fmt.Errorf("hashing local pixi.toml: %w", err)
	}
	localLockHash := store.ContentHash(string(localLock))

	result.TomlModified = project.OriginTomlHash != "" && project.OriginTomlHash != localTomlHash
	result.LockModified = project.OriginLockHash != "" && project.OriginLockHash != localLockHash

	if serverURL != "" {
		result.ServerSync = checkServerOriginStatus(s, serverURL, project)
	}

	return writeJSON(result)
}

func checkServerOriginStatus(s *store.Store, serverURL string, project *store.LocalProject) string {
	creds, err := s.LoadCredentials()
	if err != nil || creds.Token == "" {
		return "not_logged_in"
	}

	client := cliclient.New(serverURL, creds.Token)
	ctx := context.Background()

	serverProject, err := findProjectByName(client, ctx, project.OriginName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return "not_found"
		}
		return "not_reachable"
	}

	versionNumber, err := resolveVersionNumber(client, ctx, serverProject.ID, project.OriginName, project.OriginTag)
	if err != nil {
		return "tag_not_found"
	}

	toml, err := client.GetVersionPixiToml(ctx, serverProject.ID, versionNumber)
	if err != nil {
		return "not_reachable"
	}

	serverHash, err := store.TomlContentHash(toml)
	if err != nil {
		return "hash_error"
	}
	if project.OriginTomlHash != "" && project.OriginTomlHash != serverHash {
		return "server_changed"
	}

	return "in_sync"
}

func checkServerOrigin(s *store.Store, serverURL string, project *store.LocalProject) string {
	creds, err := s.LoadCredentials()
	if err != nil || creds.Token == "" {
		return "Not logged in"
	}

	client := cliclient.New(serverURL, creds.Token)
	ctx := context.Background()

	serverProject, err := findProjectByName(client, ctx, project.OriginName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return fmt.Sprintf("Project %q not found on server", project.OriginName)
		}
		return "Server not reachable"
	}

	versionNumber, err := resolveVersionNumber(client, ctx, serverProject.ID, project.OriginName, project.OriginTag)
	if err != nil {
		return fmt.Sprintf("Tag %q not found on server", project.OriginTag)
	}

	toml, err := client.GetVersionPixiToml(ctx, serverProject.ID, versionNumber)
	if err != nil {
		return "Server not reachable"
	}

	serverHash, err := store.TomlContentHash(toml)
	if err != nil {
		return fmt.Sprintf("Failed to hash server pixi.toml: %v", err)
	}
	if project.OriginTomlHash != "" && project.OriginTomlHash != serverHash {
		return fmt.Sprintf("%s:%s has changed on server since last sync", project.OriginName, project.OriginTag)
	}

	return fmt.Sprintf("In sync with %s:%s", project.OriginName, project.OriginTag)
}
