package cliclient

import "context"

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
