//go:build e2e

package main

import (
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestE2E_LocalModeNetworkGuard verifies that a real local-mode server only
// accepts requests from the local machine: loopback requests succeed, a
// request addressed to a non-local hostname is rejected, a request with a
// non-local Origin is rejected, and the listener is not reachable on
// non-loopback interfaces.
func TestE2E_LocalModeNetworkGuard(t *testing.T) {
	env := startLocalModeServer(t)

	healthURL := env.serverURL + "/api/v1/health"

	t.Run("loopback request succeeds", func(t *testing.T) {
		resp, err := http.Get(healthURL)
		if err != nil {
			t.Fatalf("GET %s: %v", healthURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("non-local Host header rejected", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, healthURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "evil.example.com"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for non-local Host, got %d", resp.StatusCode)
		}
	})

	t.Run("non-local Origin rejected", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, healthURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "https://evil.example.com")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for non-local Origin, got %d", resp.StatusCode)
		}
	})

	t.Run("listener not reachable on non-loopback interfaces", func(t *testing.T) {
		u, err := url.Parse(env.serverURL)
		if err != nil {
			t.Fatal(err)
		}
		port := u.Port()

		addrs, err := net.InterfaceAddrs()
		if err != nil {
			t.Fatalf("InterfaceAddrs: %v", err)
		}
		checked := false
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
				continue
			}
			checked = true
			target := net.JoinHostPort(ipNet.IP.String(), port)
			conn, err := net.DialTimeout("tcp", target, 2*time.Second)
			if err == nil {
				conn.Close()
				t.Fatalf("local mode server is reachable on non-loopback address %s", target)
			}
		}
		if !checked {
			t.Skip("no non-loopback IPv4 interface available")
		}
	})
}
