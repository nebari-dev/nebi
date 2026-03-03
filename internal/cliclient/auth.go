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

// DeviceCodeResponse is the response from POST /auth/cli-login/code.
type DeviceCodeResponse struct {
	Code      string `json:"code"`
	ExpiresIn int    `json:"expires_in"`
}

// DevicePollResponse is the response from GET /auth/cli-login/poll.
type DevicePollResponse struct {
	Status   string `json:"status"`
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
}

// RequestDeviceCode requests a new device code for browser-based CLI login.
func (c *Client) RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	var resp DeviceCodeResponse
	_, err := c.Post(ctx, "/auth/cli-login/code", nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	return &resp, nil
}

// PollDeviceCode checks whether the device code has been completed.
func (c *Client) PollDeviceCode(ctx context.Context, code string) (*DevicePollResponse, error) {
	var resp DevicePollResponse
	_, err := c.Get(ctx, fmt.Sprintf("/auth/cli-login/poll?code=%s", code), &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
