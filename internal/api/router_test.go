package api

import (
	"bytes"
	"encoding/json"
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
	"github.com/nebari-dev/nebi/internal/auth/authtest"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"gorm.io/gorm"
)

// buildTestRouter builds the real production router (local mode, so RBAC init is
// skipped) backed by an on-disk SQLite database, the in-memory queue, and the
// local executor. Driving the actual router exercises the real CORS middleware
// wiring and the real embedded-SPA static handler, not a hand-built stand-in.
func buildTestRouter(t *testing.T, basePath string, mutate ...func(*config.Config)) http.Handler {
	t.Helper()

	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Server.BasePath = basePath
	cfg.EncryptionKey = "test-secret-for-router-test"
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
	return NewRouter(t.Context(), cfg, database, queue.NewMemoryQueue(16), exec, nil, logger)
}

// newTestIdP starts an in-process OIDC provider whose tokens a team-mode
// router built by teamModeConfig accepts.
func newTestIdP(t *testing.T) *authtest.Server {
	t.Helper()
	idp, err := authtest.NewServer("nebi")
	if err != nil {
		t.Fatalf("start test idp: %v", err)
	}
	t.Cleanup(idp.Close)
	return idp
}

// teamModeConfig returns a team-mode config that authenticates with idp.
func teamModeConfig(t *testing.T, idp *authtest.Server, dbName string) *config.Config {
	t.Helper()
	cfg := &config.Config{Mode: config.ModeTeam}
	cfg.Auth.Type = config.AuthTypeOIDC
	cfg.EncryptionKey = "test-secret-for-team-router-test"
	cfg.Auth.OIDCIssuerURL = idp.URL
	cfg.Auth.OIDCClientID = idp.ClientID
	cfg.Auth.OIDCAdminGroups = "nebi-admin"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), dbName)
	cfg.Storage.ProjectsDir = t.TempDir()
	return cfg
}

// provisionTestUser authenticates token once, which provisions its user, and
// returns that user.
func provisionTestUser(t *testing.T, router http.Handler, database *gorm.DB, token string) models.User {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /auth/me: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var me models.User
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode /auth/me: %v", err)
	}
	var user models.User
	if err := database.First(&user, "id = ?", me.ID).Error; err != nil {
		t.Fatalf("load provisioned user: %v", err)
	}
	return user
}

func buildTeamTestRouter(t *testing.T, logger *slog.Logger) (http.Handler, string) {
	t.Helper()

	idp := newTestIdP(t)
	cfg := teamModeConfig(t, idp, "team-router-test.db")
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
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	token := idp.Token(authtest.Identity{Subject: "sub-alice", Username: "alice", Email: "alice@example.com", EmailVerified: true})
	return NewRouter(t.Context(), cfg, database, queue.NewMemoryQueue(16), exec, nil, logger), token
}

func buildLimitedLocalRouter(t *testing.T, limitCfg limits.Limits) http.Handler {
	t.Helper()

	cfg := &config.Config{Mode: config.ModeLocal, Limits: limitCfg}
	cfg.EncryptionKey = "test-secret-for-router-test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "limited-router-test.db")
	cfg.Storage.ProjectsDir = t.TempDir()
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
	return NewRouter(t.Context(), cfg, database, queue.NewMemoryQueue(16), exec, nil, logger)
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

func TestProjectCreateRejectsOversizedRequestBody(t *testing.T) {
	r := buildLimitedLocalRouter(t, limits.Limits{RequestBodyBytes: 32})

	body := `{"name":"big","pixi_toml":"` + strings.Repeat("x", 64) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
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

func TestAuthConfigAdvertisesIdentityProvider(t *testing.T) {
	idp := newTestIdP(t)
	cfg := teamModeConfig(t, idp, "auth-config.db")
	cfg.Auth.OIDCScopes = "openid, profile"
	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}
	r := NewRouter(t.Context(), cfg, database, queue.NewMemoryQueue(16), exec, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got struct {
		Type      string   `json:"type"`
		IssuerURL string   `json:"issuer_url"`
		ClientID  string   `json:"client_id"`
		Scopes    []string `json:"scopes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Type != "oidc" || got.IssuerURL != idp.URL || got.ClientID != "nebi" || strings.Join(got.Scopes, " ") != "openid profile" {
		t.Fatalf("unexpected auth config %+v", got)
	}

	// The web UI talks to the identity provider directly, so its origin must
	// be allowed by the CSP.
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "connect-src 'self' "+idp.URL) {
		t.Fatalf("expected CSP connect-src to allow the identity provider, got %q", csp)
	}
}

func TestAuthTypeNoneRunsRequestsAsAdmin(t *testing.T) {
	cfg := &config.Config{Mode: config.ModeTeam}
	cfg.Auth.Type = config.AuthTypeNone
	cfg.EncryptionKey = "test-secret-for-auth-none-router-test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "auth-none.db")
	cfg.Storage.ProjectsDir = t.TempDir()
	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}
	r := NewRouter(t.Context(), cfg, database, queue.NewMemoryQueue(16), exec, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for path, want := range map[string]string{
		"/api/v1/auth/config": `"type":"none"`,
		"/api/v1/auth/me":     `"username":"local-user"`,
		"/api/v1/admin/users": `"is_admin":true`,
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("GET %s without credentials: expected 200 with %s, got %d %s", path, want, w.Code, w.Body.String())
		}
	}
}

// TestAdminRegistryMutations_RejectConfigManaged exercises the config-managed
// registry guards (see registry.go UpdateRegistry / DeleteRegistry) through
// the real HTTP admin routes, not just the service layer.
func TestAdminRegistryMutations_RejectConfigManaged(t *testing.T) {
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.EncryptionKey = "test-secret-for-config-managed-registry-test"
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
	r := NewRouter(t.Context(), cfg, database, queue.NewMemoryQueue(16), exec, nil, logger)

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
