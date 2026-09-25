package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/pixi"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

// ErrProjectNotFound is returned when a project name is not found on the server.
var ErrProjectNotFound = errors.New("project not found on server")

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

// findProjectByName searches for a project by name on the server.
func findProjectByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Project, error) {
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	for i := range projects {
		if projects[i].Name == name {
			return &projects[i], nil
		}
	}

	return nil, fmt.Errorf("%w: %q", ErrProjectNotFound, name)
}

// validateProjectName checks that a project name doesn't contain path separators or colons,
// which would make it ambiguous with paths or server refs.
func validateProjectName(name string) error {
	return pixi.ValidateProjectName(name)
}

// lookupOrigin returns the origin fields for the current working directory project.
// Returns nil (no error) if no project is tracked or no origin is set.
func lookupOrigin() (*store.LocalProject, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	s, err := store.New()
	if err != nil {
		return nil, err
	}
	defer s.Close()

	project, err := s.FindProjectByPath(cwd)
	if err != nil {
		return nil, err
	}
	if project == nil || project.OriginName == "" {
		return nil, nil
	}

	// Sync project name if pixi.toml has changed
	if err := syncProjectName(s, project); err != nil {
		// Non-fatal: log warning but continue
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	return project, nil
}

// syncProjectName updates the stored project name if it differs from pixi.toml.
// This ensures project list shows correct names after pixi.toml edits.
func syncProjectName(s *store.Store, project *store.LocalProject) error {
	pixiTomlPath := filepath.Join(project.Path, "pixi.toml")
	content, err := os.ReadFile(pixiTomlPath)
	if err != nil {
		return nil // pixi.toml not readable, skip sync
	}

	tomlName, err := pixi.ExtractWorkspaceName(string(content))
	if err != nil {
		return err
	}

	if project.Name != tomlName {
		oldName := project.Name
		project.Name = tomlName
		if err := s.SaveProject(project); err != nil {
			return fmt.Errorf("updating project name: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Project name updated: %q -> %q (from pixi.toml)\n", oldName, tomlName)
	}

	return nil
}

// findProjectsByNameWithSync looks up projects by name. If no matches are found,
// it syncs all project names from pixi.toml (in case a rename occurred) and retries.
func findProjectsByNameWithSync(s *store.Store, name string) ([]store.LocalProject, error) {
	projects, err := s.FindProjectsByName(name)
	if err != nil {
		return nil, err
	}
	if len(projects) > 0 {
		return projects, nil
	}

	// No match — sync all project names and retry
	all, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if syncErr := syncProjectName(s, &all[i]); syncErr != nil {
			// Non-fatal: continue syncing other projects
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", all[i].Path, syncErr)
		}
	}

	return s.FindProjectsByName(name)
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

	project, err := s.FindProjectByPath(cwd)
	if err != nil {
		return err
	}
	if project == nil {
		return nil
	}

	tomlHash, err := store.TomlContentHash(tomlContent)
	if err != nil {
		return fmt.Errorf("hashing pixi.toml: %w", err)
	}

	project.OriginID = remoteID
	project.OriginName = name
	project.OriginTag = tag
	project.OriginAction = action
	project.OriginTomlHash = tomlHash
	project.OriginLockHash = store.ContentHash(lockContent)

	return s.SaveProject(project)
}

// parseProjectRef parses a reference in the format project:tag.
// Returns (project, tag) where tag may be empty if not specified.
func parseProjectRef(ref string) (string, string) {
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

// writeJSON marshals v as indented JSON to stdout.
func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// waitForProjectReady polls until the project reaches ready state or timeout.
func waitForProjectReady(client *cliclient.Client, ctx context.Context, projectID string, timeout time.Duration) (*cliclient.Project, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		project, err := client.GetProject(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get project status: %w", err)
		}
		switch project.Status {
		case "ready":
			return project, nil
		case "failed", "error":
			return nil, fmt.Errorf("project setup failed")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for project to be ready")
}
