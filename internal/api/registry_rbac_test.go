package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func buildTeamTestRouter(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()

	cfg := &config.Config{Mode: "team"}
	cfg.Auth.Type = "basic"
	cfg.Auth.JWTSecret = "test-secret-for-registry-rbac"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "registry-rbac.db")
	cfg.Storage.WorkspacesDir = t.TempDir()

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

	q := queue.NewMemoryQueue(16)
	t.Cleanup(func() { q.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(cfg, database, q, exec, nil, nil, logger), database
}

func loginTestUser(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()

	body := bytes.NewBufferString(`{"username":"` + username + `","password":"` + password + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status: got %d body %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("login response did not include token")
	}
	return resp.Token
}

func authedRequest(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestRegistryRoutesRequireRegistryRBAC(t *testing.T) {
	router, database := buildTeamTestRouter(t)

	const password = "password"
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := models.User{
		Username:     "alice",
		Email:        "alice@test.com",
		PasswordHash: passwordHash,
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	registry := models.OCIRegistry{Name: "private", URL: "https://ghcr.io", IsDefault: true}
	if err := database.Create(&registry).Error; err != nil {
		t.Fatalf("create registry: %v", err)
	}
	workspace := models.Workspace{
		Name:           "private-pub",
		OwnerID:        user.ID,
		Status:         models.WsStatusReady,
		PackageManager: "pixi",
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := database.Create(&models.WorkspaceVersion{
		WorkspaceID:   workspace.ID,
		VersionNumber: 1,
		ContentHash:   "sha-private",
		CreatedBy:     user.ID,
	}).Error; err != nil {
		t.Fatalf("create workspace version: %v", err)
	}
	if err := rbac.NewDefaultProvider().GrantWorkspaceAccess(user.ID, workspace.ID, "owner"); err != nil {
		t.Fatalf("grant workspace access: %v", err)
	}
	publication := models.Publication{
		WorkspaceID:   workspace.ID,
		VersionNumber: 1,
		RegistryID:    registry.ID,
		Repository:    "private-pub",
		Tag:           "v1",
		PublishedBy:   user.ID,
	}
	if err := database.Create(&publication).Error; err != nil {
		t.Fatalf("create publication: %v", err)
	}

	token := loginTestUser(t, router, user.Username, password)

	listResp := authedRequest(router, http.MethodGet, "/api/v1/registries", token, "")
	if listResp.Code != http.StatusOK {
		t.Fatalf("GET /registries status: got %d body %s", listResp.Code, listResp.Body.String())
	}
	var registries []map[string]any
	if err := json.Unmarshal(listResp.Body.Bytes(), &registries); err != nil {
		t.Fatalf("decode registries response: %v", err)
	}
	if len(registries) != 0 {
		t.Fatalf("expected unreadable registries to be hidden, got %+v", registries)
	}

	publicationsResp := authedRequest(router, http.MethodGet, "/api/v1/workspaces/"+workspace.ID.String()+"/publications", token, "")
	if publicationsResp.Code != http.StatusOK {
		t.Fatalf("GET /publications status: got %d body %s", publicationsResp.Code, publicationsResp.Body.String())
	}
	var publications []map[string]any
	if err := json.Unmarshal(publicationsResp.Body.Bytes(), &publications); err != nil {
		t.Fatalf("decode publications response: %v", err)
	}
	if len(publications) != 0 {
		t.Fatalf("expected unreadable publications to be hidden, got %+v", publications)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "browse repositories",
			method: http.MethodGet,
			path:   "/api/v1/registries/" + registry.ID.String() + "/repositories",
		},
		{
			name:   "list tags",
			method: http.MethodGet,
			path:   "/api/v1/registries/" + registry.ID.String() + "/tags?repo=private-pub",
		},
		{
			name:   "import",
			method: http.MethodPost,
			path:   "/api/v1/registries/" + registry.ID.String() + "/import",
			body:   `{"repository":"private-pub","tag":"v1","name":"imported"}`,
		},
		{
			name:   "publish defaults",
			method: http.MethodGet,
			path:   "/api/v1/workspaces/" + workspace.ID.String() + "/publish-defaults",
		},
		{
			name:   "publish",
			method: http.MethodPost,
			path:   "/api/v1/workspaces/" + workspace.ID.String() + "/publish",
			body:   `{"registry_id":"` + registry.ID.String() + `","repository":"private-pub","tag":"v1"}`,
		},
		{
			name:   "visibility",
			method: http.MethodPatch,
			path:   "/api/v1/workspaces/" + workspace.ID.String() + "/publications/" + publication.ID.String(),
			body:   `{"is_public":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := authedRequest(router, tt.method, tt.path, token, tt.body)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s %s: got %d body %s", tt.method, tt.path, w.Code, w.Body.String())
			}
		})
	}

	var updated models.Publication
	if err := database.First(&updated, publication.ID).Error; err != nil {
		t.Fatalf("reload publication: %v", err)
	}
	if updated.IsPublic {
		t.Fatal("publication visibility changed despite missing registry write grant")
	}
}
