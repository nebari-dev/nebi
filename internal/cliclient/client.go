// Package cliclient provides a lightweight HTTP client for the Nebi API.
package cliclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a lightweight HTTP client for the Nebi API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a new API client.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL + "/api/v1",
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, wrapConnectionError(err, c.serverURL())
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

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, wrapConnectionError(err, c.serverURL())
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

// serverURL returns the base server URL (without the /api/v1 suffix).
func (c *Client) serverURL() string {
	return strings.TrimSuffix(c.baseURL, "/api/v1")
}

// ConnectionError wraps a network error with a helpful message when the
// server is unreachable.
type ConnectionError struct {
	ServerURL string
	Err       error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf(
		"server at %s is unreachable: %v\n\nHint: If the server should be running, check that it is started.\n  To run this command in local mode instead, use the --local flag.\n  To remove the configured server, run: nebi logout",
		e.ServerURL, e.Err,
	)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// IsConnectionError returns true if the error is a ConnectionError.
func IsConnectionError(err error) bool {
	var connErr *ConnectionError
	return errors.As(err, &connErr)
}

// wrapConnectionError checks if err is a network-level connection failure
// and wraps it with a helpful message. Non-connection errors pass through.
func wrapConnectionError(err error, serverURL string) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		var opErr *net.OpError
		if errors.As(urlErr, &opErr) {
			return &ConnectionError{ServerURL: serverURL, Err: err}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return &ConnectionError{ServerURL: serverURL, Err: err}
		}
		// DNS resolution failures
		var dnsErr *net.DNSError
		if errors.As(urlErr, &dnsErr) {
			return &ConnectionError{ServerURL: serverURL, Err: err}
		}
	}
	return err
}

// IsOIDCRedirect returns true if the error indicates the server is behind an
// OIDC proxy that redirected to a login page instead of returning JSON.
// This typically manifests as a JSON decode error when the response body
// starts with '<' (HTML) instead of '{' (JSON).
func IsOIDCRedirect(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid character '<'") ||
		strings.Contains(msg, "invalid character '&lt;'")
}
