package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/localstore"
)

// resolveServerFlag returns the server argument, falling back to the default server from config.
func resolveServerFlag(serverArg string) (string, error) {
	if serverArg != "" {
		return serverArg, nil
	}
	cfg, err := localstore.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	if cfg.DefaultServer == "" {
		return "", fmt.Errorf("no server specified; use -s <server> or set a default with 'nebi server add <name> <url>'")
	}
	return cfg.DefaultServer, nil
}

// getAuthenticatedClient resolves a server name or URL, loads credentials, and returns an authenticated API client.
func getAuthenticatedClient(serverArg string) (*cliclient.Client, error) {
	serverURL, err := resolveServerURL(serverArg)
	if err != nil {
		return nil, err
	}

	creds, err := localstore.LoadCredentials()
	if err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}

	cred, ok := creds.Servers[serverURL]
	if !ok {
		return nil, fmt.Errorf("not logged in to %s; run 'nebi login %s' first", serverURL, serverArg)
	}

	return cliclient.New(serverURL, cred.Token), nil
}

// findEnvByName searches for an environment by name on the server.
func findEnvByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Environment, error) {
	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}

	for i := range envs {
		if envs[i].Name == name {
			return &envs[i], nil
		}
	}

	return nil, fmt.Errorf("environment %q not found on server", name)
}

// findGlobalWorkspaceByName looks up a global workspace by name in the local index.
func findGlobalWorkspaceByName(idx *localstore.Index, name string) *localstore.Workspace {
	for _, ws := range idx.Workspaces {
		if ws.Name == name && ws.Global {
			return ws
		}
	}
	return nil
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
	return nil
}

// parseEnvRef parses a reference in the format env:tag.
// Returns (env, tag) where tag may be empty if not specified.
func parseEnvRef(ref string) (string, string) {
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

// waitForEnvReady polls until the environment reaches ready state or timeout.
func waitForEnvReady(client *cliclient.Client, ctx context.Context, envID string, timeout time.Duration) (*cliclient.Environment, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		env, err := client.GetEnvironment(ctx, envID)
		if err != nil {
			return nil, fmt.Errorf("failed to get environment status: %w", err)
		}
		switch env.Status {
		case "ready":
			return env, nil
		case "failed", "error":
			return nil, fmt.Errorf("environment setup failed")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for environment to be ready")
}
