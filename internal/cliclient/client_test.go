package cliclient

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
)

func TestWrapConnectionError_ConnectionRefused(t *testing.T) {
	// Simulate a "connection refused" error chain: url.Error -> net.OpError
	inner := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("connect: connection refused"),
	}
	urlErr := &url.Error{
		Op:  "Post",
		URL: "http://localhost:8460/api/v1/admin/registries",
		Err: inner,
	}

	wrapped := wrapConnectionError(urlErr, "http://localhost:8460")

	var connErr *ConnectionError
	if !errors.As(wrapped, &connErr) {
		t.Fatalf("expected ConnectionError, got %T: %v", wrapped, wrapped)
	}
	if connErr.ServerURL != "http://localhost:8460" {
		t.Errorf("expected server URL http://localhost:8460, got %s", connErr.ServerURL)
	}
	if !errors.As(connErr, &connErr) {
		t.Error("IsConnectionError should return true")
	}

	// Verify the message includes hints
	msg := connErr.Error()
	for _, want := range []string{"unreachable", "--local", "nebi logout"} {
		if !contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestWrapConnectionError_DNSError(t *testing.T) {
	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "badhost.example.com",
	}
	urlErr := &url.Error{
		Op:  "Get",
		URL: "http://badhost.example.com/api/v1/workspaces",
		Err: dnsErr,
	}

	wrapped := wrapConnectionError(urlErr, "http://badhost.example.com")

	if !IsConnectionError(wrapped) {
		t.Fatalf("expected ConnectionError for DNS error, got %T: %v", wrapped, wrapped)
	}
}

func TestWrapConnectionError_NonNetworkError(t *testing.T) {
	plainErr := fmt.Errorf("some other error")
	result := wrapConnectionError(plainErr, "http://localhost:8460")

	if IsConnectionError(result) {
		t.Fatal("plain error should not be wrapped as ConnectionError")
	}
	if result != plainErr {
		t.Error("non-network error should pass through unchanged")
	}
}

func TestWrapConnectionError_APIError(t *testing.T) {
	apiErr := &APIError{StatusCode: 500, Body: "internal server error"}
	result := wrapConnectionError(apiErr, "http://localhost:8460")

	if IsConnectionError(result) {
		t.Fatal("API error should not be wrapped as ConnectionError")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
