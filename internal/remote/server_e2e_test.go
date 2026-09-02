//go:build e2e

package remote

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/server"
)

// startSpikeServer boots a private nebi server in team mode (real
// authentication — local mode bypasses auth entirely, which would make
// the bad-credentials conformance step meaningless) with a fake pixi
// binary, mirroring cmd/nebi's e2e TestMain. Admin credentials are
// admin/adminpass.
func startSpikeServer(t *testing.T) string {
	t.Helper()

	env := map[string]string{
		"NEBI_MODE":                   "team",
		"NEBI_DATABASE_DRIVER":        "sqlite",
		"NEBI_DATABASE_DSN":           filepath.Join(t.TempDir(), "remote-spike.db"),
		"NEBI_STORAGE_WORKSPACES_DIR": t.TempDir(),
		"NEBI_QUEUE_TYPE":             "memory",
		"NEBI_AUTH_JWT_SECRET":        "e2e-test-secret-that-is-at-least-32-chars-long",
		"NEBI_LOG_LEVEL":              "error",
		"NEBI_DATABASE_LOG_LEVEL":     "silent",
		"ADMIN_USERNAME":              "admin",
		"ADMIN_PASSWORD":              "adminpass",
	}

	pixiPath := filepath.Join(t.TempDir(), "pixi")
	pixiScript := `#!/bin/sh
if [ "${1:-}" = "--version" ]; then printf 'pixi 0.0.0-test\n'; exit 0; fi
if [ "${1:-}" = "lock" ]; then [ -f pixi.lock ] || printf 'version: 6\n' > pixi.lock; exit 0; fi
if [ "${1:-}" = "install" ]; then mkdir -p .pixi/envs/default; exit 0; fi
exit 0
`
	if err := os.WriteFile(pixiPath, []byte(pixiScript), 0o755); err != nil {
		t.Fatalf("write fake pixi: %v", err)
	}
	env["NEBI_PIXI_PATH"] = pixiPath

	port, err := findFreePort()
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	env["NEBI_SERVER_PORT"] = fmt.Sprintf("%d", port)

	for k, v := range env {
		t.Setenv(k, v)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx, server.Config{
			Port:    port,
			Mode:    "both",
			Version: "remote-spike-e2e",
		})
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(baseURL + "/api/v1/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			t.Fatal("server never became healthy")
		}
		select {
		case err := <-serverErr:
			t.Fatalf("server exited early: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	return baseURL
}

func findFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port, nil
}

// TestServerRemote_Conformance runs the exact same lifecycle script the
// OCI remote passes, against a real in-process nebi server.
func TestServerRemote_Conformance(t *testing.T) {
	baseURL := startSpikeServer(t)
	r := &ServerRemote{BaseURL: baseURL}

	good := AuthenticationCredential{
		Type:     BasicAuthenticationCredentialType,
		Username: "admin",
		Password: "adminpass",
	}
	bad := AuthenticationCredential{
		Type:     BasicAuthenticationCredentialType,
		Username: "admin",
		Password: "wrong",
	}
	runRemoteConformance(t, r, good, bad, nonexistentUUID)
}

// TestServerRemote_TokenAuth covers the second credential type: reuse a
// JWT minted by a basic login as a token credential on a fresh remote.
func TestServerRemote_TokenAuth(t *testing.T) {
	baseURL := startSpikeServer(t)
	ctx := context.Background()

	login, err := cliclient.NewWithoutAuth(baseURL).Login(ctx, "admin", "adminpass")
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	second := &ServerRemote{BaseURL: baseURL}
	err = second.Authenticate(ctx, AuthenticationCredential{
		Type:  TokenAuthenticationCredentialType,
		Token: login.Token,
	})
	if err != nil {
		t.Fatalf("token authenticate: %v", err)
	}
	if _, err := second.List(ctx); err != nil {
		t.Fatalf("List with token credential: %v", err)
	}
}
