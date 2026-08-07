package cliclient

import (
	"context"
	"net/url"
)

// ListBuildEnvVars returns current-user build environment variable metadata.
func (c *Client) ListBuildEnvVars(ctx context.Context) ([]BuildEnvVar, error) {
	var vars []BuildEnvVar
	_, err := c.Get(ctx, "/build-env-vars", &vars)
	if err != nil {
		return nil, err
	}
	return vars, nil
}

// UpsertBuildEnvVar creates or updates a current-user build environment variable.
func (c *Client) UpsertBuildEnvVar(ctx context.Context, req UpsertBuildEnvVarRequest) (*BuildEnvVar, error) {
	var envVar BuildEnvVar
	_, err := c.Put(ctx, "/build-env-vars", req, &envVar)
	if err != nil {
		return nil, err
	}
	return &envVar, nil
}

// DeleteBuildEnvVar deletes a current-user build environment variable by key.
func (c *Client) DeleteBuildEnvVar(ctx context.Context, key string) error {
	_, err := c.Delete(ctx, "/build-env-vars/"+url.PathEscape(key))
	return err
}
