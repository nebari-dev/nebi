package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
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
		remote.GET("/server", h.GetServer)
		remote.DELETE("/server", h.DisconnectServer)
		remote.GET("/workspaces", h.ListWorkspaces)
		remote.GET("/workspaces/:id", h.GetWorkspace)
		remote.POST("/workspaces", h.CreateWorkspace)
		remote.DELETE("/workspaces/:id", h.DeleteWorkspace)
		remote.GET("/workspaces/:id/versions", h.ListVersions)
		remote.GET("/workspaces/:id/tags", h.ListTags)
		remote.GET("/workspaces/:id/pixi-toml", h.GetPixiToml)
		remote.GET("/workspaces/:id/versions/:version/pixi-toml", h.GetVersionPixiToml)
		remote.GET("/workspaces/:id/versions/:version/pixi-lock", h.GetVersionPixiLock)
		remote.POST("/workspaces/:id/push", h.PushVersion)
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

func TestListWorkspaces_NotConnected(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/workspaces", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConnectServer_WithMockRemote(t *testing.T) {
	// Create a mock remote Nebi server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/auth/login" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"token": "test-token-abc",
				"user": map[string]any{
					"username": "remoteuser",
					"id":       "user-123",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	router := setupRouter(db)

	body := `{"url":"` + mockServer.URL + `","username":"remoteuser","password":"secret"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/remote/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	if resp["url"] != mockServer.URL {
		t.Errorf("expected url=%s, got %v", mockServer.URL, resp["url"])
	}
	if resp["username"] != "remoteuser" {
		t.Errorf("expected username=remoteuser, got %v", resp["username"])
	}

	// Verify credentials were stored in DB
	var cfg store.Config
	db.First(&cfg)
	if cfg.ServerURL != mockServer.URL {
		t.Errorf("expected stored server_url=%s, got %q", mockServer.URL, cfg.ServerURL)
	}
	var creds store.Credentials
	db.First(&creds)
	if creds.Token != "test-token-abc" {
		t.Errorf("expected stored token=test-token-abc, got %q", creds.Token)
	}
	if creds.Username != "remoteuser" {
		t.Errorf("expected stored username=remoteuser, got %q", creds.Username)
	}
}
