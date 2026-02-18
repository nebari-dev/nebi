package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/pkgmgr/pixi"
	"github.com/nebari-dev/nebi/internal/store"
)

// ErrWsNotFound is returned when a workspace name is not found on the server.
var ErrWsNotFound = errors.New("workspace not found on server")

// getAuthenticatedClient loads credentials and returns an authenticated API client.
func getAuthenticatedClient() (*cliclient.Client, error) {
	s, err := store.New()
	if err != nil {
		return nil, err
	}
	defer s.Close()

	serverURL, err := s.LoadServerURL()
	if err != nil {
		return nil, fmt.Errorf("loading server URL: %w", err)
	}
	if serverURL == "" {
		return nil, fmt.Errorf("no server configured; run 'nebi login <server-url>' first")
	}

	creds, err := s.LoadCredentials()
	if err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}
	if creds.Token == "" {
		return nil, fmt.Errorf("not logged in; run 'nebi login <server-url>' first")
	}

	return cliclient.New(serverURL, creds.Token), nil
}

// findWsByName searches for a workspace by name on the server.
func findWsByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Workspace, error) {
	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}

	for i := range workspaces {
		if workspaces[i].Name == name {
			return &workspaces[i], nil
		}
	}

	return nil, fmt.Errorf("%w: %q", ErrWsNotFound, name)
}

// validateWorkspaceName checks that a workspace name doesn't contain path separators or colons,
// which would make it ambiguous with paths or server refs.
func validateWorkspaceName(name string) error {
	if strings.ContainsAny(name, `/\:`) {
		return fmt.Errorf("workspace name %q must not contain '/', '\\', or ':'", name)
	}
	if name == "" {
		return fmt.Errorf("workspace name must not be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("workspace name %q is reserved and cannot be used", name)
	}
	return nil
}

// lookupOrigin returns the origin fields for the current working directory workspace.
// Returns nil (no error) if no workspace is tracked or no origin is set.
func lookupOrigin() (*store.LocalWorkspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	s, err := store.New()
	if err != nil {
		return nil, err
	}
	defer s.Close()

	ws, err := s.FindWorkspaceByPath(cwd)
	if err != nil {
		return nil, err
	}
	if ws == nil || ws.OriginName == "" {
		return nil, nil
	}

	// Sync workspace name if pixi.toml has changed
	if err := syncWorkspaceName(s, ws); err != nil {
		// Non-fatal: log warning but continue
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	return ws, nil
}

// syncWorkspaceName updates the stored workspace name if it differs from pixi.toml.
// This ensures workspace list shows correct names after pixi.toml edits.
func syncWorkspaceName(s *store.Store, ws *store.LocalWorkspace) error {
	pixiTomlPath := filepath.Join(ws.Path, "pixi.toml")
	content, err := os.ReadFile(pixiTomlPath)
	if err != nil {
		return nil // pixi.toml not readable, skip sync
	}

	tomlName, err := pixi.ExtractWorkspaceName(string(content))
	if err != nil {
		return nil // Can't extract name, skip sync
	}

	if ws.Name != tomlName {
		if err := validateWorkspaceName(tomlName); err != nil {
			return fmt.Errorf("pixi.toml workspace name is invalid: %w", err)
		}
		oldName := ws.Name
		ws.Name = tomlName
		if err := s.SaveWorkspace(ws); err != nil {
			return fmt.Errorf("updating workspace name: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Workspace name updated: %q -> %q (from pixi.toml)\n", oldName, tomlName)
	}

	return nil
}

// saveOrigin records a push/pull origin for the current working directory.
func saveOrigin(name, tag, action, tomlContent, lockContent string) error {
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
		return nil
	}

	tomlHash, err := store.TomlContentHash(tomlContent)
	if err != nil {
		return fmt.Errorf("hashing pixi.toml: %w", err)
	}

	ws.OriginName = name
	ws.OriginTag = tag
	ws.OriginAction = action
	ws.OriginTomlHash = tomlHash
	ws.OriginLockHash = store.ContentHash(lockContent)

	return s.SaveWorkspace(ws)
}

// parseWsRef parses a reference in the format workspace:tag.
// Returns (workspace, tag) where tag may be empty if not specified.
func parseWsRef(ref string) (string, string) {
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}

// formatTimestamp parses an ISO 8601 timestamp and returns a human-friendly format.
func formatTimestamp(ts string) string {
	t, err := time.Parse("2006-01-02T15:04:05Z", ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04")
}

// waitForWsReady polls until the workspace reaches ready state or timeout.
func waitForWsReady(client *cliclient.Client, ctx context.Context, wsID string, timeout time.Duration) (*cliclient.Workspace, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ws, err := client.GetWorkspace(ctx, wsID)
		if err != nil {
			return nil, fmt.Errorf("failed to get workspace status: %w", err)
		}
		switch ws.Status {
		case "ready":
			return ws, nil
		case "failed", "error":
			return nil, fmt.Errorf("workspace setup failed")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for workspace to be ready")
}
