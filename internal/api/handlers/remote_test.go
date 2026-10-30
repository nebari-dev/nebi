package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/auth/authtest"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB creates an in-memory SQLite DB with the store tables seeded.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&store.Config{}, &store.Credentials{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	db.Exec("INSERT OR IGNORE INTO store_config (id) VALUES (1)")
	db.Exec("INSERT OR IGNORE INTO store_credentials (id) VALUES (1)")
	return db
}

// setupRouter creates a Gin engine with the remote handler routes registered.
func setupRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewRemoteHandler(db)
	remote := r.Group("/api/v1/remote")
	{
		remote.POST("/connect", h.ConnectServer)
		remote.POST("/connect/poll", h.PollConnect)
		remote.GET("/server", h.GetServer)
		remote.DELETE("/server", h.DisconnectServer)
		remote.GET("/projects", h.ListProjects)
		remote.GET("/projects/:id", h.GetProject)
		remote.POST("/projects", h.CreateProject)
		remote.DELETE("/projects/:id", h.DeleteProject)
		remote.GET("/projects/:id/versions", h.ListVersions)
		remote.GET("/projects/:id/tags", h.ListTags)
		remote.GET("/projects/:id/pixi-toml", h.GetPixiToml)
		remote.GET("/projects/:id/versions/:version/pixi-toml", h.GetVersionPixiToml)
		remote.GET("/projects/:id/versions/:version/pixi-lock", h.GetVersionPixiLock)
		remote.POST("/projects/:id/push", h.PushVersion)
		remote.GET("/registries", h.ListRegistries)
		remote.GET("/jobs", h.ListJobs)
		remote.POST("/admin/registries", h.CreateAdminRegistry)
		remote.PUT("/admin/registries/:id", h.UpdateAdminRegistry)
		remote.DELETE("/admin/registries/:id", h.DeleteAdminRegistry)
	}
	return r
}

func TestGetServer_NoConfig(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/server", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "disconnected" {
		t.Errorf("expected status=disconnected, got %v", resp["status"])
	}
}

func TestConnectServer_MissingFields(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/remote/connect", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDisconnectServer(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// First set some data in the store
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", "http://example.com")
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "some-token",
		"username": "someuser",
	})

	// Disconnect
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/remote/server", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "disconnected" {
		t.Errorf("expected status=disconnected, got %v", resp["status"])
	}

	// Verify DB was cleared
	var cfg store.Config
	db.First(&cfg)
	if cfg.ServerURL != "" {
		t.Errorf("expected empty server_url, got %q", cfg.ServerURL)
	}
	var creds store.Credentials
	db.First(&creds)
	if creds.Token != "" {
		t.Errorf("expected empty token, got %q", creds.Token)
	}
}

func TestGetServer_AfterStoreSetup(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Set config and credentials in DB
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", "https://nebi.example.com")
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/server", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "connected" {
		t.Errorf("expected status=connected, got %v", resp["status"])
	}
	if resp["url"] != "https://nebi.example.com" {
		t.Errorf("expected url=https://nebi.example.com, got %v", resp["url"])
	}
	if resp["username"] != "testuser" {
		t.Errorf("expected username=testuser, got %v", resp["username"])
	}
}

func TestListProjects_NotConnected(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/projects", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

// fakeRemote is a remote nebi server whose API accepts access tokens from idp
// (or any request when idp is nil, i.e. auth disabled).
type fakeRemote struct {
	*httptest.Server
	idp       *authtest.Server
	lastToken string
}

func newFakeRemote(t *testing.T, idp *authtest.Server) *fakeRemote {
	t.Helper()
	f := &fakeRemote{idp: idp}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/config":
			if f.idp == nil {
				_ = json.NewEncoder(w).Encode(map[string]any{"type": "none"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type": "oidc", "issuer_url": f.idp.URL, "client_id": f.idp.ClientID,
				"scopes": []string{"openid", "profile"},
			})
		case "/api/v1/auth/me":
			f.lastToken = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if f.idp != nil && f.lastToken == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing authorization"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-123", "username": "remoteuser"})
		case "/api/v1/projects":
			f.lastToken = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func doJSON(t *testing.T, router *gin.Engine, method, path, body string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// pollUntilDone polls the pending connection until the device-token wait has
// finished, which takes at least one provider poll interval after approval.
func pollUntilDone(t *testing.T, router *gin.Engine) (int, map[string]any) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		status, resp := doJSON(t, router, "POST", "/api/v1/remote/connect/poll", "")
		if status != http.StatusOK || resp["status"] != "pending" || time.Now().After(deadline) {
			return status, resp
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestConnectServer_DeviceFlow(t *testing.T) {
	idp, err := authtest.NewServer("nebi")
	if err != nil {
		t.Fatalf("start idp: %v", err)
	}
	t.Cleanup(idp.Close)
	remote := newFakeRemote(t, idp)
	db := setupTestDB(t)
	router := setupRouter(db)

	status, resp := doJSON(t, router, "POST", "/api/v1/remote/connect", `{"url":"`+remote.URL+`/"}`)
	if status != http.StatusOK {
		t.Fatalf("connect: expected 200, got %d: %v", status, resp)
	}
	userCode, _ := resp["user_code"].(string)
	if userCode == "" || resp["verification_uri"] == "" {
		t.Fatalf("expected a device code response, got %v", resp)
	}

	status, resp = doJSON(t, router, "POST", "/api/v1/remote/connect/poll", "")
	if status != http.StatusOK || resp["status"] != "pending" {
		t.Fatalf("expected pending, got %d %v", status, resp)
	}

	if err := idp.ApproveDevice(userCode, authtest.Identity{Subject: "sub-1", Username: "remoteuser"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	status, resp = pollUntilDone(t, router)
	if status != http.StatusOK || resp["status"] != "connected" || resp["username"] != "remoteuser" || resp["url"] != remote.URL {
		t.Fatalf("expected connected, got %d %v", status, resp)
	}

	var cfg store.Config
	db.First(&cfg)
	var creds store.Credentials
	db.First(&creds)
	if cfg.ServerURL != remote.URL || creds.Token == "" || creds.RefreshToken == "" ||
		creds.TokenURL != idp.URL+"/token" || creds.ClientID != "nebi" || creds.TokenExpiry == nil || creds.Username != "remoteuser" {
		t.Fatalf("unexpected stored connection %q %+v", cfg.ServerURL, creds)
	}

	// The connection is used for proxied calls; once the access token is
	// about to expire it is refreshed and the rotated tokens are stored.
	if status, _ := doJSON(t, router, "GET", "/api/v1/remote/projects", ""); status != http.StatusOK {
		t.Fatalf("list projects: %d", status)
	}
	if remote.lastToken != creds.Token {
		t.Fatal("expected the stored access token to be sent")
	}
	expired := time.Now().Add(-time.Minute)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Update("token_expiry", expired)
	router = setupRouter(db) // fresh handler: no cached token source
	if status, _ := doJSON(t, router, "GET", "/api/v1/remote/projects", ""); status != http.StatusOK {
		t.Fatalf("list projects after expiry: %d", status)
	}
	var refreshed store.Credentials
	db.First(&refreshed)
	if refreshed.Token == creds.Token || refreshed.RefreshToken == creds.RefreshToken || remote.lastToken != refreshed.Token {
		t.Fatalf("expected refreshed and persisted tokens, got %+v", refreshed)
	}

	// Nothing is pending any more.
	if status, _ := doJSON(t, router, "POST", "/api/v1/remote/connect/poll", ""); status != http.StatusConflict {
		t.Fatalf("expected 409 without a pending connection, got %d", status)
	}
}

func TestConnectServer_DeviceFlowDenied(t *testing.T) {
	idp, err := authtest.NewServer("nebi")
	if err != nil {
		t.Fatalf("start idp: %v", err)
	}
	t.Cleanup(idp.Close)
	remote := newFakeRemote(t, idp)
	db := setupTestDB(t)
	router := setupRouter(db)

	_, resp := doJSON(t, router, "POST", "/api/v1/remote/connect", `{"url":"`+remote.URL+`"}`)
	if err := idp.DenyDevice(resp["user_code"].(string)); err != nil {
		t.Fatalf("deny: %v", err)
	}
	status, resp := pollUntilDone(t, router)
	if status != http.StatusBadRequest || !strings.Contains(resp["error"].(string), "denied") {
		t.Fatalf("expected 400 access denied, got %d %v", status, resp)
	}
	var creds store.Credentials
	db.First(&creds)
	if creds.LoggedIn() {
		t.Fatalf("expected no stored login, got %+v", creds)
	}
}

func TestConnectServer_AuthDisabledRemote(t *testing.T) {
	remote := newFakeRemote(t, nil)
	db := setupTestDB(t)
	router := setupRouter(db)

	status, resp := doJSON(t, router, "POST", "/api/v1/remote/connect", `{"url":"`+remote.URL+`"}`)
	if status != http.StatusOK || resp["status"] != "connected" || resp["username"] != "remoteuser" {
		t.Fatalf("expected immediate connection, got %d %v", status, resp)
	}
	_, resp = doJSON(t, router, "GET", "/api/v1/remote/server", "")
	if resp["status"] != "connected" {
		t.Fatalf("expected connected status, got %v", resp)
	}
	if status, _ := doJSON(t, router, "GET", "/api/v1/remote/projects", ""); status != http.StatusOK || remote.lastToken != "" {
		t.Fatalf("expected an unauthenticated proxied call, got %d with token %q", status, remote.lastToken)
	}
}

func TestConnectServer_RejectsNonHTTPURL(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)
	if status, _ := doJSON(t, router, "POST", "/api/v1/remote/connect", `{"url":"ftp://example.com"}`); status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", status)
	}
}

func TestListRegistries_NotConnected(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/registries", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when not connected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListRegistries_WithMockRemote(t *testing.T) {
	// Create a mock remote Nebi server that returns registries
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/registries" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":         "reg-1",
					"name":       "Docker Hub",
					"url":        "https://registry-1.docker.io",
					"is_default": true,
				},
				{
					"id":         "reg-2",
					"name":       "GHCR",
					"url":        "https://ghcr.io",
					"is_default": false,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)

	// Set up connection to mock server
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", mockServer.URL)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/registries", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var registries []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &registries); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(registries) != 2 {
		t.Errorf("expected 2 registries, got %d", len(registries))
	}
	if registries[0]["name"] != "Docker Hub" {
		t.Errorf("expected first registry name=Docker Hub, got %v", registries[0]["name"])
	}
}

func TestListVersions_WithMockRemotePreservesManifestVersion(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/projects/ws-1/versions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":               "version-1",
					"project_id":       "ws-1",
					"version_number":   1,
					"manifest_version": "0.0.3",
					"description":      "Initial project creation",
					"created_at":       "2026-08-14T07:35:38Z",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", mockServer.URL)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/projects/ws-1/versions", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var versions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &versions); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if versions[0]["manifest_version"] != "0.0.3" {
		t.Errorf("expected manifest_version=0.0.3, got %v", versions[0]["manifest_version"])
	}
	if versions[0]["description"] != "Initial project creation" {
		t.Errorf("expected description to be preserved, got %v", versions[0]["description"])
	}
}

func TestCreateAdminRegistry_NotConnected(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	body := `{"name":"GHCR","url":"ghcr.io"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/remote/admin/registries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when not connected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAdminRegistry_WithMockRemote(t *testing.T) {
	var received map[string]any
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/admin/registries" {
			json.NewDecoder(r.Body).Decode(&received)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{
				"id":         "reg-1",
				"name":       received["name"],
				"url":        received["url"],
				"is_default": false,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", mockServer.URL)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	body := `{"name":"GHCR","url":"ghcr.io","api_token":"secret-token"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/remote/admin/registries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["name"] != "GHCR" {
		t.Errorf("expected name=GHCR, got %v", resp["name"])
	}

	// The remote server must actually receive the api_token field - it must
	// not be silently dropped by the proxy's request struct.
	if received["api_token"] != "secret-token" {
		t.Errorf("expected remote server to receive api_token=secret-token, got %v", received["api_token"])
	}
}

func TestCreateAdminRegistry_PreservesDisplayFields(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/admin/registries" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{
				"id":             "reg-1",
				"name":           "GHCR",
				"url":            "ghcr.io",
				"namespace":      "nebari",
				"has_api_token":  true,
				"config_managed": true,
				"is_default":     false,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", mockServer.URL)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	body := `{"name":"GHCR","url":"ghcr.io"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/remote/admin/registries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// The proxy must not strip fields the UI relies on. config_managed drives
	// the "Managed" badge and disables edit/delete; dropping it makes managed
	// remote registries look editable. namespace/has_api_token are shown too.
	if resp["config_managed"] != true {
		t.Errorf("expected config_managed=true to survive the proxy, got %v", resp["config_managed"])
	}
	if resp["has_api_token"] != true {
		t.Errorf("expected has_api_token=true to survive the proxy, got %v", resp["has_api_token"])
	}
	if resp["namespace"] != "nebari" {
		t.Errorf("expected namespace=nebari to survive the proxy, got %v", resp["namespace"])
	}
}

func TestUpdateAdminRegistry_NotConnected(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	body := `{"name":"GHCR"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/remote/admin/registries/reg-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when not connected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateAdminRegistry_WithMockRemote(t *testing.T) {
	var received map[string]any
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && r.URL.Path == "/api/v1/admin/registries/reg-1" {
			json.NewDecoder(r.Body).Decode(&received)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"id":         "reg-1",
				"name":       received["name"],
				"url":        "ghcr.io",
				"is_default": false,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", mockServer.URL)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	body := `{"name":"GHCR2","api_token":"new-token"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/remote/admin/registries/reg-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// The remote server must actually receive api_token on the update path too -
	// it must not be silently dropped by the proxy's UpdateRegistryRequest struct.
	if received["api_token"] != "new-token" {
		t.Errorf("expected remote server to receive api_token=new-token, got %v", received["api_token"])
	}
}

func TestDeleteAdminRegistry_NotConnected(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/remote/admin/registries/reg-1", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when not connected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteAdminRegistry_WithMockRemote(t *testing.T) {
	var deletedPath string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/api/v1/admin/registries/reg-1" {
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", mockServer.URL)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/remote/admin/registries/reg-1", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if deletedPath != "/api/v1/admin/registries/reg-1" {
		t.Errorf("expected proxy to DELETE the registry on the remote, got path %q", deletedPath)
	}
}

func TestListJobs_NotConnected(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/jobs", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when not connected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListJobs_WithMockRemote(t *testing.T) {
	// Create a mock remote Nebi server that returns jobs
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/jobs" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":         "job-1",
					"project_id": "ws-1",
					"type":       "create",
					"status":     "completed",
					"created_at": "2024-01-01T00:00:00Z",
				},
				{
					"id":         "job-2",
					"project_id": "ws-2",
					"type":       "install",
					"status":     "running",
					"created_at": "2024-01-02T00:00:00Z",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)

	// Set up connection to mock server
	db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", mockServer.URL)
	db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "valid-token",
		"username": "testuser",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/jobs", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var jobs []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0]["type"] != "create" {
		t.Errorf("expected first job type=create, got %v", jobs[0]["type"])
	}
	if jobs[1]["status"] != "running" {
		t.Errorf("expected second job status=running, got %v", jobs[1]["status"])
	}
}
