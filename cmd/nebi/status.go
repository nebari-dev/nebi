package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace sync status",
	Long: `Show the current workspace's tracking info and sync status with servers.

Displays the workspace name, type, path, and origin info for each server
that has been pushed to or pulled from.

If a server is reachable, checks whether the local files or server version
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

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	ws, exists := idx.Workspaces[cwd]
	if !exists {
		fmt.Fprintln(os.Stderr, "Not a tracked workspace. Run 'nebi init'.")
		return nil
	}

	wsType := "local"
	if ws.Global {
		wsType = "global"
	}
	fmt.Fprintf(os.Stdout, "Workspace: %s\n", ws.Name)
	fmt.Fprintf(os.Stdout, "Type:      %s\n", wsType)
	fmt.Fprintf(os.Stdout, "Path:      %s\n", ws.Path)

	if len(ws.Origins) == 0 {
		fmt.Fprintln(os.Stdout, "\nNo origins. Push or pull to set an origin.")
		return nil
	}

	// Check local file modifications against stored hashes
	localToml, _ := os.ReadFile(filepath.Join(cwd, "pixi.toml"))
	localLock, _ := os.ReadFile(filepath.Join(cwd, "pixi.lock"))
	localTomlHash, err := localstore.TomlContentHash(string(localToml))
	if err != nil {
		return fmt.Errorf("hashing local pixi.toml: %w", err)
	}
	localLockHash := localstore.ContentHash(string(localLock))

	// Use first origin's hashes as reference for local modification detection
	var refOrigin *localstore.Origin
	for _, o := range ws.Origins {
		refOrigin = o
		break
	}

	fmt.Fprintln(os.Stdout)
	if refOrigin != nil {
		if refOrigin.TomlHash != "" && refOrigin.TomlHash != localTomlHash {
			fmt.Fprintln(os.Stdout, "pixi.toml modified locally")
		}
		if refOrigin.LockHash != "" && refOrigin.LockHash != localLockHash {
			fmt.Fprintln(os.Stdout, "pixi.lock modified locally")
		}
	}

	fmt.Fprintln(os.Stdout, "\nOrigins:")
	for serverName, origin := range ws.Origins {
		fmt.Fprintf(os.Stdout, "  %s → %s:%s (%s, %s)\n", serverName, origin.Name, origin.Tag, origin.Action, origin.Timestamp)

		// Try to check server status
		serverStatus := checkServerOrigin(serverName, origin)
		if serverStatus != "" {
			fmt.Fprintf(os.Stdout, "    %s\n", serverStatus)
		}
	}

	return nil
}

// checkServerOrigin attempts to reach the server and compare the origin hash.
func checkServerOrigin(serverName string, origin *localstore.Origin) string {
	serverURL, err := resolveServerURL(serverName)
	if err != nil {
		return fmt.Sprintf("Server %q is not reachable", serverName)
	}

	creds, err := localstore.LoadCredentials()
	if err != nil {
		return fmt.Sprintf("Server %q is not reachable", serverName)
	}

	cred, ok := creds.Servers[serverURL]
	if !ok {
		return "Not logged in"
	}

	client := cliclient.New(serverURL, cred.Token)

	ctx := context.Background()
	env, err := findEnvByName(client, ctx, origin.Name)
	if err != nil {
		if errors.Is(err, ErrEnvNotFound) {
			return fmt.Sprintf("Workspace %q not found on server", origin.Name)
		}
		return fmt.Sprintf("Server %q is not reachable", serverName)
	}

	versionNumber, err := resolveVersionNumber(client, ctx, env.ID, origin.Name, origin.Tag)
	if err != nil {
		return fmt.Sprintf("Tag %q not found on server", origin.Tag)
	}

	toml, err := client.GetVersionPixiToml(ctx, env.ID, versionNumber)
	if err != nil {
		return fmt.Sprintf("Server %q is not reachable", serverName)
	}

	serverHash, err := localstore.TomlContentHash(toml)
	if err != nil {
		return fmt.Sprintf("Failed to hash server pixi.toml: %v", err)
	}
	if origin.TomlHash != "" && origin.TomlHash != serverHash {
		return fmt.Sprintf("%s:%s has changed on server since last sync", origin.Name, origin.Tag)
	}

	return fmt.Sprintf("In sync with %s:%s", origin.Name, origin.Tag)
}
