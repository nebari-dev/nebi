package cliclient

import (
	"context"
	"fmt"
)

// GetServerVersion calls GET /version (public, no auth required).
func (c *Client) GetServerVersion(ctx context.Context) (*ServerVersion, error) {
	var sv ServerVersion
	_, err := c.Get(ctx, "/version", &sv)
	if err != nil {
		return nil, err
	}
	return &sv, nil
}

// GetCurrentUser calls GET /auth/me to get the authenticated user.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	var user User
	_, err := c.Get(ctx, "/auth/me", &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// AuthConfig is the response from GET /auth/config.
type AuthConfig struct {
	Type      string   `json:"type"`
	IssuerURL string   `json:"issuer_url,omitempty"`
	ClientID  string   `json:"client_id,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
}

// Supported AuthConfig types.
const (
	AuthTypeOIDC = "oidc"
	AuthTypeNone = "none"
)

// GetAuthConfig calls GET /auth/config (public) to learn how the server
// authenticates requests.
func (c *Client) GetAuthConfig(ctx context.Context) (*AuthConfig, error) {
	var cfg AuthConfig
	if _, err := c.Get(ctx, "/auth/config", &cfg); err != nil {
		return nil, fmt.Errorf("fetching auth config: %w", err)
	}
	return &cfg, nil
}
