package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/pixi"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

// ErrWsNotFound is returned when a workspace name is not found on the server.
var ErrWsNotFound = errors.New("workspace not found on server")

// getAuthenticatedClient loads credentials and returns an authenticated API client.
func getAuthenticatedClient() (*cliclient.Client, error) {
	// Check environment variables first
	if envToken := os.Getenv("NEBI_AUTH_TOKEN"); envToken != "" {
		if envURL := os.Getenv("NEBI_REMOTE_URL"); envURL != "" {
			return cliclient.New(envURL, envToken), nil
		}
	}

	// Fall back to SQLite store
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

// isLocalMode returns true if the command should operate in local mode.
// Local mode is used when:
// 1. The --local flag is set, OR
// 2. No server is configured and user is not logged in
func isLocalMode(cmd *cobra.Command) bool {
	local, _ := cmd.Flags().GetBool("local")
	if local {
		return true
	}

	s, err := store.New()
	if err != nil {
		return true
	}
	defer s.Close()

	serverURL, err := s.LoadServerURL()
	if err != nil || serverURL == "" {
		return true
	}

	creds, err := s.LoadCredentials()
	if err != nil || creds.Token == "" {
		return true
	}

	return false
}

// findWsByName accepts id::<uuid> or an unambiguous display name.
func findWsByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Workspace, error) {
	if id, explicit, err := workspaceID(name); explicit {
		if err != nil {
			return nil, err
		}
		return client.GetWorkspace(ctx, id.String())
	}
	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	var match *cliclient.Workspace
	var ids []string
	for i := range workspaces {
		if workspaces[i].Name == name {
			match = &workspaces[i]
			ids = append(ids, "id::"+match.ID)
		}
	}
	if len(ids) > 1 {
		return nil, fmt.Errorf("multiple workspaces named %q; use an id::<uuid> selector: %s", name, strings.Join(ids, ", "))
	}
	if match == nil {
		return nil, fmt.Errorf("%w: %q", ErrWsNotFound, name)
	}
	return match, nil
}

// validateWorkspaceName checks that a workspace name doesn't contain path separators or colons,
// which would make it ambiguous with paths or server refs.
func validateWorkspaceName(name string) error {
	return pixi.ValidateWorkspaceName(name)
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

	return ws, nil
}

// saveOrigin records a push/pull origin for the current working directory.
func saveOrigin(remoteID, name, tag, action, tomlContent, lockContent string) error {
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

	// Update only origin fields; a concurrently chosen display name is unrelated.
	return s.DB().Model(&store.LocalWorkspace{}).Where("id = ?", ws.ID).Updates(map[string]interface{}{
		"origin_id":        remoteID,
		"origin_name":      name,
		"origin_tag":       tag,
		"origin_action":    action,
		"origin_toml_hash": tomlHash,
		"origin_lock_hash": store.ContentHash(lockContent),
	}).Error
}

// parseWsRef parses a reference in the format workspace:tag.
// Returns (workspace, tag) where tag may be empty if not specified.
func parseWsRef(ref string) (string, string) {
	if idx := strings.LastIndex(ref, ":"); idx != -1 && !(strings.HasPrefix(ref, "id::") && idx == 3) {
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

// writeJSON marshals v as indented JSON to stdout.
func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
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

// workspaceID reserves the colon-containing id:: prefix, which cannot be a
// workspace name. Bare UUID-shaped names retain their original meaning.
func workspaceID(ref string) (uuid.UUID, bool, error) {
	if !strings.HasPrefix(ref, "id::") {
		return uuid.Nil, false, nil
	}
	id, err := uuid.Parse(strings.TrimPrefix(ref, "id::"))
	if err != nil {
		return uuid.Nil, true, fmt.Errorf("invalid workspace ID selector %q: %w", ref, err)
	}
	return id, true, nil
}

// findLocalWorkspaces resolves an explicit ID or returns all name matches so
// interactive callers can keep their existing duplicate-name picker.
func findLocalWorkspaces(s *store.Store, ref string) ([]store.LocalWorkspace, error) {
	if id, explicit, err := workspaceID(ref); explicit {
		if err != nil {
			return nil, err
		}
		ws, err := s.GetWorkspace(id)
		if err != nil {
			return nil, err
		}
		return []store.LocalWorkspace{*ws}, nil
	}
	return s.FindWorkspacesByName(ref)
}
