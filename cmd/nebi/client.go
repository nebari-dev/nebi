package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/localstore"
)

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

// parseEnvRef parses a reference in the format env:tag.
// Returns (env, tag) where tag may be empty if not specified.
func parseEnvRef(ref string) (string, string) {
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}
