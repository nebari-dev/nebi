package cliclient

import (
	"context"
	"fmt"
	"net/url"
)

// ListProjects returns all projects.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	_, err := c.Get(ctx, "/projects", &projects)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProject returns a project by ID.
func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var project Project
	_, err := c.Get(ctx, fmt.Sprintf("/projects/%s", id), &project)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// CreateProject creates a new project.
func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error) {
	var project Project
	_, err := c.Post(ctx, "/projects", req, &project)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// DeleteProject deletes a project by ID.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	_, err := c.Delete(ctx, fmt.Sprintf("/projects/%s", id))
	return err
}

// GetProjectPackages returns packages for a project.
func (c *Client) GetProjectPackages(ctx context.Context, projectID string) ([]Package, error) {
	var pkgs []Package
	_, err := c.Get(ctx, fmt.Sprintf("/projects/%s/packages", projectID), &pkgs)
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

// GetProjectPublications returns publications for a project.
func (c *Client) GetProjectPublications(ctx context.Context, projectID string) ([]Publication, error) {
	var pubs []Publication
	_, err := c.Get(ctx, fmt.Sprintf("/projects/%s/publications", projectID), &pubs)
	if err != nil {
		return nil, err
	}
	return pubs, nil
}

// GetPublishDefaults returns suggested defaults for publishing a project.
func (c *Client) GetPublishDefaults(ctx context.Context, projectID string, registryID ...string) (*PublishDefaults, error) {
	var defaults PublishDefaults
	path := fmt.Sprintf("/projects/%s/publish-defaults", projectID)
	if len(registryID) > 0 && registryID[0] != "" {
		path += "?registry_id=" + url.QueryEscape(registryID[0])
	}
	_, err := c.Get(ctx, path, &defaults)
	if err != nil {
		return nil, err
	}
	return &defaults, nil
}

// GetProjectVersions returns versions for a project.
func (c *Client) GetProjectVersions(ctx context.Context, projectID string) ([]ProjectVersion, error) {
	var versions []ProjectVersion
	_, err := c.Get(ctx, fmt.Sprintf("/projects/%s/versions", projectID), &versions)
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// GetVersionPixiToml returns the pixi.toml for a specific version.
func (c *Client) GetVersionPixiToml(ctx context.Context, projectID string, version int32) (string, error) {
	content, _, err := c.GetText(ctx, fmt.Sprintf("/projects/%s/versions/%d/pixi-toml", projectID, version))
	if err != nil {
		return "", err
	}
	return content, nil
}

// GetVersionPixiLock returns the pixi.lock for a specific version.
func (c *Client) GetVersionPixiLock(ctx context.Context, projectID string, version int32) (string, error) {
	content, _, err := c.GetText(ctx, fmt.Sprintf("/projects/%s/versions/%d/pixi-lock", projectID, version))
	if err != nil {
		return "", err
	}
	return content, nil
}

// GetProjectTags returns server-side tags for a project.
func (c *Client) GetProjectTags(ctx context.Context, projectID string) ([]ProjectTag, error) {
	var tags []ProjectTag
	_, err := c.Get(ctx, fmt.Sprintf("/projects/%s/tags", projectID), &tags)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// PushVersion pushes a new version to the server with a tag.
func (c *Client) PushVersion(ctx context.Context, projectID string, req PushRequest) (*PushResponse, error) {
	var resp PushResponse
	_, err := c.Post(ctx, fmt.Sprintf("/projects/%s/push", projectID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// PublishProject publishes a project to a registry.
func (c *Client) PublishProject(ctx context.Context, projectID string, req PublishRequest) (*PublishResponse, error) {
	var resp PublishResponse
	_, err := c.Post(ctx, fmt.Sprintf("/projects/%s/publish", projectID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// InstallProject queues an environment install for a project.
// Returns the queued Job (install runs asynchronously on the server).
func (c *Client) InstallProject(ctx context.Context, projectID string) (*Job, error) {
	var job Job
	_, err := c.Post(ctx, fmt.Sprintf("/projects/%s/install", projectID), nil, &job)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// UninstallProject queues removal of a project's installed environment.
func (c *Client) UninstallProject(ctx context.Context, projectID string) (*Job, error) {
	var job Job
	_, err := c.Post(ctx, fmt.Sprintf("/projects/%s/uninstall", projectID), nil, &job)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// RollbackProject queues a server-side rollback to a previous version.
// Returns the queued Job (rollback runs asynchronously on the server).
func (c *Client) RollbackProject(ctx context.Context, projectID string, versionNumber int) (*Job, error) {
	req := RollbackRequest{VersionNumber: versionNumber}
	var job Job
	_, err := c.Post(ctx, fmt.Sprintf("/projects/%s/rollback", projectID), req, &job)
	if err != nil {
		return nil, err
	}
	return &job, nil
}
