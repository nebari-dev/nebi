package cliclient

import (
	"context"
	"fmt"
)

// ListEnvironments returns all environments.
func (c *Client) ListEnvironments(ctx context.Context) ([]Environment, error) {
	var envs []Environment
	_, err := c.Get(ctx, "/environments", &envs)
	if err != nil {
		return nil, err
	}
	return envs, nil
}

// GetEnvironment returns an environment by ID.
func (c *Client) GetEnvironment(ctx context.Context, id string) (*Environment, error) {
	var env Environment
	_, err := c.Get(ctx, fmt.Sprintf("/environments/%s", id), &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

// CreateEnvironment creates a new environment.
func (c *Client) CreateEnvironment(ctx context.Context, req CreateEnvironmentRequest) (*Environment, error) {
	var env Environment
	_, err := c.Post(ctx, "/environments", req, &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

// DeleteEnvironment deletes an environment by ID.
func (c *Client) DeleteEnvironment(ctx context.Context, id string) error {
	_, err := c.Delete(ctx, fmt.Sprintf("/environments/%s", id))
	return err
}

// GetEnvironmentPackages returns packages for an environment.
func (c *Client) GetEnvironmentPackages(ctx context.Context, envID string) ([]Package, error) {
	var pkgs []Package
	_, err := c.Get(ctx, fmt.Sprintf("/environments/%s/packages", envID), &pkgs)
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

// GetEnvironmentPublications returns publications for an environment.
func (c *Client) GetEnvironmentPublications(ctx context.Context, envID string) ([]Publication, error) {
	var pubs []Publication
	_, err := c.Get(ctx, fmt.Sprintf("/environments/%s/publications", envID), &pubs)
	if err != nil {
		return nil, err
	}
	return pubs, nil
}

// GetEnvironmentVersions returns versions for an environment.
func (c *Client) GetEnvironmentVersions(ctx context.Context, envID string) ([]EnvironmentVersion, error) {
	var versions []EnvironmentVersion
	_, err := c.Get(ctx, fmt.Sprintf("/environments/%s/versions", envID), &versions)
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// GetVersionPixiToml returns the pixi.toml for a specific version.
func (c *Client) GetVersionPixiToml(ctx context.Context, envID string, version int32) (string, error) {
	content, _, err := c.GetText(ctx, fmt.Sprintf("/environments/%s/versions/%d/pixi-toml", envID, version))
	if err != nil {
		return "", err
	}
	return content, nil
}

// GetVersionPixiLock returns the pixi.lock for a specific version.
func (c *Client) GetVersionPixiLock(ctx context.Context, envID string, version int32) (string, error) {
	content, _, err := c.GetText(ctx, fmt.Sprintf("/environments/%s/versions/%d/pixi-lock", envID, version))
	if err != nil {
		return "", err
	}
	return content, nil
}

// PublishEnvironment publishes an environment to a registry.
func (c *Client) PublishEnvironment(ctx context.Context, envID string, req PublishRequest) (*PublishResponse, error) {
	var resp PublishResponse
	_, err := c.Post(ctx, fmt.Sprintf("/environments/%s/publish", envID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
