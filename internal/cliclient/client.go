// Package cliclient provides a lightweight HTTP client for the Nebi API.
package cliclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// Client is a lightweight HTTP client for the Nebi API.
type Client struct {
	baseURL    string
	tokens     oauth2.TokenSource // nil: unauthenticated
	httpClient *http.Client
}

// New creates a new API client that sends a fixed bearer token.
func New(baseURL, token string) *Client {
	var tokens oauth2.TokenSource
	if token != "" {
		tokens = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	return NewWithTokenSource(baseURL, tokens)
}

// NewWithTokenSource creates a new API client that asks tokens for a bearer
// token before every request, so an expiring login can be refreshed.
func NewWithTokenSource(baseURL string, tokens oauth2.TokenSource) *Client {
	return &Client{
		baseURL: baseURL + "/api/v1",
		tokens:  tokens,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// authorize sets the Authorization header when the client has credentials.
func (c *Client) authorize(req *http.Request) error {
	if c.tokens == nil {
		return nil
	}
	tok, err := c.tokens.Token()
	if err != nil {
		return err
	}
	if tok.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}
	return nil
}

// NewWithoutAuth creates a new API client without authentication (for login).
func NewWithoutAuth(baseURL string) *Client {
	return &Client{
		baseURL: baseURL + "/api/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// request performs an HTTP request and decodes the JSON response.
func (c *Client) request(ctx context.Context, method, path string, body, result interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if err := c.authorize(req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return resp, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return resp, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp, nil
}

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, path string, result interface{}) (*http.Response, error) {
	return c.request(ctx, http.MethodGet, path, nil, result)
}

// Post performs a POST request.
func (c *Client) Post(ctx context.Context, path string, body, result interface{}) (*http.Response, error) {
	return c.request(ctx, http.MethodPost, path, body, result)
}

// Put performs a PUT request.
func (c *Client) Put(ctx context.Context, path string, body, result interface{}) (*http.Response, error) {
	return c.request(ctx, http.MethodPut, path, body, result)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) (*http.Response, error) {
	return c.request(ctx, http.MethodDelete, path, nil, nil)
}

// GetText performs a GET request and returns the response as a string.
func (c *Client) GetText(ctx context.Context, path string) (string, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	if err := c.authorize(req); err != nil {
		return "", nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", resp, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	return string(body), resp, nil
}

// APIError represents an API error response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Body)
}

// IsNotFound returns true if the error is a 404 Not Found error.
func IsNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 404
	}
	return false
}

// IsForbidden returns true if the error is a 403 Forbidden error.
func IsForbidden(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 403
	}
	return false
}

// IsUnauthorized returns true if the error is a 401 Unauthorized error.
func IsUnauthorized(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 401
	}
	return false
}
