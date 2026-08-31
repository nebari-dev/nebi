package remote

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
)

// ServerRemote implements Remote against a nebi server, wrapping
// internal/cliclient.
type ServerRemote struct {
	// BaseURL is the server root, e.g. "http://localhost:8080".
	BaseURL string
	// CreateReadyTimeout bounds how long Push waits for a newly
	// created workspace to become ready before pushing its version
	// (workspace creation is asynchronous on the server — a spike
	// finding). Zero means 60s.
	CreateReadyTimeout time.Duration

	client *cliclient.Client // set by a successful Authenticate
}

var _ Remote = (*ServerRemote)(nil)

func (r *ServerRemote) SupportedAuthentication() []AuthenticationCredentialType {
	// The server additionally supports an OIDC device flow (probed via
	// GET /auth/device-config); that cannot be expressed as a static
	// credential and is out of scope for the spike (see README).
	return []AuthenticationCredentialType{
		BasicAuthenticationCredentialType,
		TokenAuthenticationCredentialType,
	}
}

func (r *ServerRemote) Authenticate(ctx context.Context, ac AuthenticationCredential) error {
	if err := ac.Validate(); err != nil {
		return err
	}
	switch ac.Type {
	case BasicAuthenticationCredentialType:
		resp, err := cliclient.NewWithoutAuth(r.BaseURL).Login(ctx, ac.Username, ac.Password)
		if err != nil {
			return mapServerError(err)
		}
		r.client = cliclient.New(r.BaseURL, resp.Token)
	case TokenAuthenticationCredentialType:
		c := cliclient.New(r.BaseURL, ac.Token)
		if _, err := c.GetCurrentUser(ctx); err != nil {
			return mapServerError(err)
		}
		r.client = c
	default:
		return fmt.Errorf("server remote: unsupported auth type %q", ac.Type)
	}
	return nil
}

func (r *ServerRemote) List(ctx context.Context) ([]WorkspacePreview, error) {
	if r.client == nil {
		return nil, ErrNotAuthenticated
	}
	wss, err := r.client.ListWorkspaces(ctx)
	if err != nil {
		return nil, mapServerError(err)
	}
	var previews []WorkspacePreview
	for _, ws := range wss {
		tags, err := r.client.GetWorkspaceTags(ctx, ws.ID)
		if err != nil {
			return nil, mapServerError(err)
		}
		p := WorkspacePreview{ID: ws.ID, Name: ws.Name}
		for _, t := range tags {
			if t.Tag == latestTag {
				p.LatestRef = latestTag
				continue
			}
			p.Refs = append(p.Refs, t.Tag)
		}
		previews = append(previews, p)
	}
	return previews, nil
}

func (r *ServerRemote) Pull(ctx context.Context, id string) (Workspace, error) {
	if r.client == nil {
		return Workspace{}, ErrNotAuthenticated
	}
	wsID, ref := splitIDRef(id)
	ws, err := r.client.GetWorkspace(ctx, wsID)
	if err != nil {
		return Workspace{}, mapServerError(err)
	}

	version, resolvedRef, err := r.resolveRef(ctx, wsID, ref)
	if err != nil {
		return Workspace{}, err
	}
	toml, err := r.client.GetVersionPixiToml(ctx, wsID, version)
	if err != nil {
		return Workspace{}, mapServerError(err)
	}
	lock, err := r.client.GetVersionPixiLock(ctx, wsID, version)
	if err != nil {
		return Workspace{}, mapServerError(err)
	}
	return Workspace{
		WorkspacePreview: WorkspacePreview{ID: ws.ID, Name: ws.Name},
		Ref:              resolvedRef,
		PixiToml:         toml,
		PixiLock:         lock,
	}, nil
}

// resolveRef maps a version ref (tag) to a server version number. An
// empty ref resolves to the "latest" tag, falling back to the highest
// version number for workspaces that were created but never pushed to.
func (r *ServerRemote) resolveRef(ctx context.Context, wsID, ref string) (int32, string, error) {
	tags, err := r.client.GetWorkspaceTags(ctx, wsID)
	if err != nil {
		return 0, "", mapServerError(err)
	}
	lookup := ref
	if lookup == "" {
		lookup = latestTag
	}
	for _, t := range tags {
		if t.Tag == lookup {
			return int32(t.VersionNumber), lookup, nil
		}
	}
	if ref == "" {
		versions, err := r.client.GetWorkspaceVersions(ctx, wsID)
		if err != nil {
			return 0, "", mapServerError(err)
		}
		var max int32
		for _, v := range versions {
			if v.VersionNumber > max {
				max = v.VersionNumber
			}
		}
		if max > 0 {
			return max, "", nil
		}
	}
	return 0, "", fmt.Errorf("%w: ref %q on workspace %s", ErrNotFound, lookup, wsID)
}

// Push creates a new version tagged ws.Ref. With an empty ws.ID a new
// workspace named ws.Name is created first; because server-side
// creation is asynchronous (a job moves it pending → ready), Push polls
// until the workspace is ready before pushing the version.
func (r *ServerRemote) Push(ctx context.Context, ws Workspace) error {
	if r.client == nil {
		return ErrNotAuthenticated
	}
	if ws.Ref == "" {
		return errors.New("server remote: push requires Ref")
	}
	if ws.Ref == latestTag {
		return fmt.Errorf("server remote: %q is reserved", latestTag)
	}

	wsID := ws.ID
	if wsID == "" {
		if ws.Name == "" {
			return errors.New("server remote: push requires ID or Name")
		}
		created, err := r.client.CreateWorkspace(ctx, cliclient.CreateWorkspaceRequest{
			Name:     ws.Name,
			PixiToml: &ws.PixiToml,
		})
		if err != nil {
			return mapServerError(err)
		}
		wsID = created.ID
		if err := r.waitReady(ctx, wsID); err != nil {
			return err
		}
	}

	_, err := r.client.PushVersion(ctx, wsID, cliclient.PushRequest{
		Tag:      ws.Ref,
		PixiToml: ws.PixiToml,
		PixiLock: ws.PixiLock,
	})
	return mapServerError(err)
}

func (r *ServerRemote) waitReady(ctx context.Context, wsID string) error {
	timeout := r.CreateReadyTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		ws, err := r.client.GetWorkspace(ctx, wsID)
		if err != nil {
			return mapServerError(err)
		}
		switch ws.Status {
		case "ready":
			return nil
		case "failed":
			return fmt.Errorf("server remote: workspace %s failed to create", wsID)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("server remote: workspace %s not ready after %s (status %q)", wsID, timeout, ws.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// mapServerError folds cliclient HTTP failures into the package
// sentinels.
func mapServerError(err error) error {
	if err == nil {
		return nil
	}
	var ae *cliclient.APIError
	if errors.As(err, &ae) {
		switch ae.StatusCode {
		case 401:
			return fmt.Errorf("%w: %v", ErrUnauthorized, err)
		case 403:
			return fmt.Errorf("%w: %v", ErrForbidden, err)
		case 404:
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		case 409:
			return fmt.Errorf("%w: %v", ErrConflict, err)
		}
	}
	return err
}
