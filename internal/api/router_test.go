package api

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
)

// buildTestRouter builds the real production router (local mode, so RBAC init is
// skipped) backed by an on-disk SQLite database, the in-memory queue, and the
// local executor. Driving the actual router exercises the real CORS middleware
// wiring and the real embedded-SPA static handler, not a hand-built stand-in.
func buildTestRouter(t *testing.T, basePath string, mutate ...func(*config.Config)) http.Handler {
	t.Helper()

	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Server.BasePath = basePath
	cfg.Auth.JWTSecret = "test-secret-for-router-test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "router-test.db")
	cfg.Registries.SeedDefault = true
	for _, m := range mutate {
		m(cfg)
	}

	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database, cfg.Registries.SeedDefault); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(cfg, database, queue.NewMemoryQueue(16), exec, nil, nil, logger)
}

func buildTeamTestRouter(t *testing.T, logger *slog.Logger) (http.Handler, string) {
	t.Helper()

	const (
		username  = "alice"
		password  = "correct-horse-battery-staple"
		jwtSecret = "test-secret-for-team-router-test"
	)

	cfg := &config.Config{Mode: config.ModeTeam}
	cfg.Auth.Type = "basic"
	cfg.Auth.JWTSecret = jwtSecret
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "team-router-test.db")
	cfg.Registries.SeedDefault = true

	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database, cfg.Registries.SeedDefault); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := models.User{
		Username:     username,
		Email:        username + "@example.com",
		PasswordHash: hash,
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	authenticator, err := auth.NewBasicAuthenticator(database, jwtSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	login, err := authenticator.Login(username, password)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return NewRouter(cfg, database, queue.NewMemoryQueue(16), exec, nil, nil, logger), login.Token
}

func buildLimitedLocalRouter(t *testing.T, limitCfg limits.Limits) http.Handler {
	t.Helper()

	cfg := &config.Config{Mode: config.ModeLocal, Limits: limitCfg}
	cfg.Auth.JWTSecret = "test-secret-for-router-test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "limited-router-test.db")
	cfg.Storage.WorkspacesDir = t.TempDir()
	cfg.Registries.SeedDefault = true

	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database, cfg.Registries.SeedDefault); err != nil {
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

func TestWorkspaceCreateRejectsOversizedRequestBody(t *testing.T) {
	r := buildLimitedLocalRouter(t, limits.Limits{RequestBodyBytes: 32})

	body := `{"name":"big","pixi_toml":"` + strings.Repeat("x", 64) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized body, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProtectedRoutesRejectQueryToken(t *testing.T) {
	r, token := buildTeamTestRouter(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected bearer header to authenticate, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me?token="+url.QueryEscape(token), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected query token to be rejected with 401, got %d", w.Code)
	}
}

func TestLoggingMiddlewareOmitsQueryString(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(loggingMiddleware())
	r.GET("/api/v1/auth/me", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me?token=secret-bearer-material", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	output := logs.String()
	if !strings.Contains(output, "/api/v1/auth/me") {
		t.Fatalf("expected log output to include request path, got %q", output)
	}
	if strings.Contains(output, "token") || strings.Contains(output, "secret-bearer-material") {
		t.Fatalf("expected log output to omit query string, got %q", output)
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

// TestAdminRegistryMutations_RejectConfigManaged exercises the config-managed
// registry guards (see registry.go UpdateRegistry / DeleteRegistry) through
// the real HTTP admin routes, not just the service layer.
func TestAdminRegistryMutations_RejectConfigManaged(t *testing.T) {
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Auth.JWTSecret = "test-secret-for-config-managed-registry-test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "config-managed-registry-test.db")
	cfg.Registries.SeedDefault = false

	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database, cfg.Registries.SeedDefault); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewRouter(cfg, database, queue.NewMemoryQueue(16), exec, nil, nil, logger)

	managed := models.OCIRegistry{ID: uuid.New(), Name: "managed", URL: "a.io", ConfigManaged: true}
	if err := database.Create(&managed).Error; err != nil {
		t.Fatalf("seed config-managed registry: %v", err)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/registries/"+managed.ID.String(), strings.NewReader(`{"url":"b.io"}`))
	putReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, putReq)
	if w.Code != http.StatusConflict {
		t.Fatalf("PUT: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "managed by server configuration") {
		t.Fatalf("PUT: expected body to mention server configuration, got %s", w.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/registries/"+managed.ID.String(), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, deleteReq)
	if w.Code != http.StatusConflict {
		t.Fatalf("DELETE: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "managed by server configuration") {
		t.Fatalf("DELETE: expected body to mention server configuration, got %s", w.Body.String())
	}
}

// TestCORSAllowsConfiguredOrigin drives the real router with an
// operator-configured non-loopback origin (server.allowed_origins) and
// asserts the CORS layer echoes it. Browsers require a matching
// Access-Control-Allow-Origin for the SPA's crossorigin module bundle when
// nebi is served behind a reverse proxy such as jupyter-server-proxy.
func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	router := buildTestRouter(t, "", func(cfg *config.Config) {
		cfg.Server.AllowedOrigins = "https://hub.example.com"
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://hub.example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://hub.example.com" {
		t.Errorf("expected configured origin echoed in Access-Control-Allow-Origin, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for unlisted origin, got %q", got)
	}
}
