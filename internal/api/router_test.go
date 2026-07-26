package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/queue"
)

// buildTestRouter builds the real production router (local mode, so RBAC init is
// skipped) backed by an on-disk SQLite database, the in-memory queue, and the
// local executor. Driving the actual router exercises the real CORS middleware
// wiring and the real embedded-SPA static handler, not a hand-built stand-in.
func buildTestRouter(t *testing.T, basePath string) http.Handler {
	t.Helper()

	cfg := &config.Config{Mode: "local"}
	cfg.Server.BasePath = basePath
	cfg.Auth.JWTSecret = "test-secret-for-router-test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "router-test.db")

	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(cfg, database, queue.NewMemoryQueue(16), exec, nil, nil, logger)
}

func TestCORSMiddlewareNoInvalidCredentialedWildcard(t *testing.T) {
	r := buildTestRouter(t, "")

	// /api/v1/health is a real public route; /assets/* flows through the real
	// SPA static handler. Both pass through the global CORS middleware.
	// The router is in local mode, where the allowed origin is echoed for
	// local UIs (e.g. the Vite dev server) instead of a wildcard.
	const origin = "http://localhost:8461"
	for _, path := range []string{"/api/v1/health", "/assets/index-abc123.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		acao := w.Header().Get("Access-Control-Allow-Origin")
		acac := w.Header().Get("Access-Control-Allow-Credentials")

		if acao == "*" && acac == "true" {
			t.Fatalf("%s: invalid CORS combo: ACAO=%q ACAC=%q (wildcard origin cannot be credentialed)", path, acao, acac)
		}
		if acao != origin {
			t.Fatalf("%s: expected Access-Control-Allow-Origin %q, got %q", path, origin, acao)
		}
		if acac != "" {
			t.Fatalf("%s: expected no Access-Control-Allow-Credentials, got %q", path, acac)
		}
	}
}

// TestLegacyCLILoginRoutesRemoved is the regression test for issue #448: the
// legacy device-code CLI login flow silently authorized the CLI from an
// existing proxy session cookie on a bare GET, with no confirmation step
// (CSRF) and no single-use enforcement on the completed code. Nothing in the
// shipped CLI uses it (nebi login only speaks the RFC 8628 device flow at
// /auth/device-config and /auth/device-token), so the fix removes the routes
// outright rather than hardening a flow with no legitimate caller.
func TestLegacyCLILoginRoutesRemoved(t *testing.T) {
	r := buildTestRouter(t, "")

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/auth/cli-login?code=ABCD-1234"},
		{http.MethodPost, "/api/v1/auth/cli-login/code"},
		{http.MethodGet, "/api/v1/auth/cli-login/poll?code=ABCD-1234"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s: expected 404 (route removed), got %d", tc.method, tc.path, w.Code)
		}
	}
}
