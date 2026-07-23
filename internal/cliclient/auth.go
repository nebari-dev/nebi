package cliclient

import (
	"context"
	"fmt"
)

// Login authenticates with the server and returns a token.
func (c *Client) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	req := LoginRequest{
		Username: username,
		Password: password,
	}

	var resp LoginResponse
	_, err := c.Post(ctx, "/auth/login", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

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

// DeviceConfigResponse is the response from GET /auth/device-config.
type DeviceConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	IssuerURL string `json:"issuer_url,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
}

// GetDeviceConfig fetches the OIDC device flow configuration from the server.
func (c *Client) GetDeviceConfig(ctx context.Context) (*DeviceConfigResponse, error) {
	var resp DeviceConfigResponse
	_, err := c.Get(ctx, "/auth/device-config", &resp)
	if err != nil {
		return nil, fmt.Errorf("fetching device config: %w", err)
	}
	return &resp, nil
}

// DeviceTokenResponse is the response from POST /auth/device-token.
type DeviceTokenResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// ExchangeDeviceToken exchanges a Keycloak ID token for a Nebi JWT.
func (c *Client) ExchangeDeviceToken(ctx context.Context, idToken string) (*DeviceTokenResponse, error) {
	var resp DeviceTokenResponse
	_, err := c.Post(ctx, "/auth/device-token", map[string]string{"id_token": idToken}, &resp)
	if err != nil {
		return nil, fmt.Errorf("exchanging device token: %w", err)
	}
	return &resp, nil
}
