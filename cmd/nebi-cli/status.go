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
	Workspace    string `json:"workspace"`
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
	Short: "Show workspace sync status",
	Long: `Show the current workspace's tracking info and sync status with the server.

Displays the workspace name, path, and origin info for the
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

	ws, err := s.FindWorkspaceByPath(cwd)
	if err != nil {
		return err
	}
	if ws == nil {
		if statusJSON {
			return fmt.Errorf("not a tracked workspace")
		}
		fmt.Fprintln(os.Stderr, "Not a tracked workspace. Run 'nebi init'.")
		return nil
	}

	// Sync workspace name if pixi.toml has changed
	if err := syncWorkspaceName(s, ws); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	serverURL, _ := s.LoadServerURL()

	if statusJSON {
		return runStatusJSON(s, ws, serverURL, cwd)
	}

	fmt.Fprintf(os.Stdout, "Workspace: %s\n", ws.Name)
	fmt.Fprintf(os.Stdout, "Path:      %s\n", ws.Path)

	if serverURL != "" {
		fmt.Fprintf(os.Stdout, "Server:    %s\n", serverURL)
	} else {
		fmt.Fprintln(os.Stdout, "Server:    (not configured)")
	}

	if ws.OriginName == "" {
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

	if ws.OriginTomlHash != "" && ws.OriginTomlHash != localTomlHash {
		fmt.Fprintln(os.Stdout, "pixi.toml modified locally")
	}
	if ws.OriginLockHash != "" && ws.OriginLockHash != localLockHash {
		fmt.Fprintln(os.Stdout, "pixi.lock modified locally")
	}

	fmt.Fprintln(os.Stdout, "\nOrigin:")
	fmt.Fprintf(os.Stdout, "  %s:%s (%s)\n", ws.OriginName, ws.OriginTag, ws.OriginAction)

	if serverURL != "" {
		serverStatus := checkServerOrigin(s, serverURL, ws)
		if serverStatus != "" {
			fmt.Fprintf(os.Stdout, "  %s\n", serverStatus)
		}
	}

	return nil
}

func runStatusJSON(s *store.Store, ws *store.LocalWorkspace, serverURL, cwd string) error {
	result := statusResult{
		Workspace:    ws.Name,
		Path:         ws.Path,
		Server:       serverURL,
		OriginName:   ws.OriginName,
		OriginTag:    ws.OriginTag,
		OriginAction: ws.OriginAction,
	}

	if ws.OriginName == "" {
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

	result.TomlModified = ws.OriginTomlHash != "" && ws.OriginTomlHash != localTomlHash
	result.LockModified = ws.OriginLockHash != "" && ws.OriginLockHash != localLockHash

	if serverURL != "" {
		result.ServerSync = checkServerOriginStatus(s, serverURL, ws)
	}

	return writeJSON(result)
}

func checkServerOriginStatus(s *store.Store, serverURL string, ws *store.LocalWorkspace) string {
	creds, err := s.LoadCredentials()
	if err != nil || creds.Token == "" {
		return "not_logged_in"
	}

	client := cliclient.New(serverURL, creds.Token)
	ctx := context.Background()

	serverWs, err := findWsByName(client, ctx, ws.OriginName)
	if err != nil {
		if errors.Is(err, ErrWsNotFound) {
			return "not_found"
		}
		return "not_reachable"
	}

	versionNumber, err := resolveVersionNumber(client, ctx, serverWs.ID, ws.OriginName, ws.OriginTag)
	if err != nil {
		return "tag_not_found"
	}

	toml, err := client.GetVersionPixiToml(ctx, serverWs.ID, versionNumber)
	if err != nil {
		return "not_reachable"
	}

	serverHash, err := store.TomlContentHash(toml)
	if err != nil {
		return "hash_error"
	}
	if ws.OriginTomlHash != "" && ws.OriginTomlHash != serverHash {
		return "server_changed"
	}

	return "in_sync"
}

func checkServerOrigin(s *store.Store, serverURL string, ws *store.LocalWorkspace) string {
	creds, err := s.LoadCredentials()
	if err != nil || creds.Token == "" {
		return "Not logged in"
	}

	client := cliclient.New(serverURL, creds.Token)
	ctx := context.Background()

	serverWs, err := findWsByName(client, ctx, ws.OriginName)
	if err != nil {
		if errors.Is(err, ErrWsNotFound) {
			return fmt.Sprintf("Workspace %q not found on server", ws.OriginName)
		}
		return "Server not reachable"
	}

	versionNumber, err := resolveVersionNumber(client, ctx, serverWs.ID, ws.OriginName, ws.OriginTag)
	if err != nil {
		return fmt.Sprintf("Tag %q not found on server", ws.OriginTag)
	}

	toml, err := client.GetVersionPixiToml(ctx, serverWs.ID, versionNumber)
	if err != nil {
		return "Server not reachable"
	}

	serverHash, err := store.TomlContentHash(toml)
	if err != nil {
		return fmt.Sprintf("Failed to hash server pixi.toml: %v", err)
	}
	if ws.OriginTomlHash != "" && ws.OriginTomlHash != serverHash {
		return fmt.Sprintf("%s:%s has changed on server since last sync", ws.OriginName, ws.OriginTag)
	}

	return fmt.Sprintf("In sync with %s:%s", ws.OriginName, ws.OriginTag)
}
