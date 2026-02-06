package cliclient

import (
	"context"
	"fmt"
)

// ListWorkspaces returns all workspaces.
func (c *Client) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	var workspaces []Workspace
	_, err := c.Get(ctx, "/workspaces", &workspaces)
	if err != nil {
		return nil, err
	}
	return workspaces, nil
}

// GetWorkspace returns a workspace by ID.
func (c *Client) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	var ws Workspace
	_, err := c.Get(ctx, fmt.Sprintf("/workspaces/%s", id), &ws)
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

// CreateWorkspace creates a new workspace.
func (c *Client) CreateWorkspace(ctx context.Context, req CreateWorkspaceRequest) (*Workspace, error) {
	var ws Workspace
	_, err := c.Post(ctx, "/workspaces", req, &ws)
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

// DeleteWorkspace deletes a workspace by ID.
func (c *Client) DeleteWorkspace(ctx context.Context, id string) error {
	_, err := c.Delete(ctx, fmt.Sprintf("/workspaces/%s", id))
	return err
}

// GetWorkspacePackages returns packages for a workspace.
func (c *Client) GetWorkspacePackages(ctx context.Context, wsID string) ([]Package, error) {
	var pkgs []Package
	_, err := c.Get(ctx, fmt.Sprintf("/workspaces/%s/packages", wsID), &pkgs)
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

// GetWorkspacePublications returns publications for a workspace.
func (c *Client) GetWorkspacePublications(ctx context.Context, wsID string) ([]Publication, error) {
	var pubs []Publication
	_, err := c.Get(ctx, fmt.Sprintf("/workspaces/%s/publications", wsID), &pubs)
	if err != nil {
		return nil, err
	}
	return pubs, nil
}

// GetWorkspaceVersions returns versions for a workspace.
func (c *Client) GetWorkspaceVersions(ctx context.Context, wsID string) ([]WorkspaceVersion, error) {
	var versions []WorkspaceVersion
	_, err := c.Get(ctx, fmt.Sprintf("/workspaces/%s/versions", wsID), &versions)
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// GetVersionPixiToml returns the pixi.toml for a specific version.
func (c *Client) GetVersionPixiToml(ctx context.Context, wsID string, version int32) (string, error) {
	content, _, err := c.GetText(ctx, fmt.Sprintf("/workspaces/%s/versions/%d/pixi-toml", wsID, version))
	if err != nil {
		return "", err
	}
	return content, nil
}

// GetVersionPixiLock returns the pixi.lock for a specific version.
func (c *Client) GetVersionPixiLock(ctx context.Context, wsID string, version int32) (string, error) {
	content, _, err := c.GetText(ctx, fmt.Sprintf("/workspaces/%s/versions/%d/pixi-lock", wsID, version))
	if err != nil {
		return "", err
	}
	return content, nil
}

// GetWorkspaceTags returns server-side tags for a workspace.
func (c *Client) GetWorkspaceTags(ctx context.Context, wsID string) ([]WorkspaceTag, error) {
	var tags []WorkspaceTag
	_, err := c.Get(ctx, fmt.Sprintf("/workspaces/%s/tags", wsID), &tags)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// PushVersion pushes a new version to the server with a tag.
func (c *Client) PushVersion(ctx context.Context, wsID string, req PushRequest) (*PushResponse, error) {
	var resp PushResponse
	_, err := c.Post(ctx, fmt.Sprintf("/workspaces/%s/push", wsID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// PublishWorkspace publishes a workspace to a registry.
func (c *Client) PublishWorkspace(ctx context.Context, wsID string, req PublishRequest) (*PublishResponse, error) {
	var resp PublishResponse
	_, err := c.Post(ctx, fmt.Sprintf("/workspaces/%s/publish", wsID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
