package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/models"
	serverapi "github.com/nebari-dev/nebi/internal/server/api"
	"github.com/nebari-dev/nebi/internal/server/core"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type openRBAC struct{}

func (openRBAC) CanReadProject(uuid.UUID, uuid.UUID) (bool, error)           { return true, nil }
func (openRBAC) CanWriteProject(uuid.UUID, uuid.UUID) (bool, error)          { return true, nil }
func (openRBAC) CanReadRegistry(uuid.UUID, uuid.UUID) (bool, error)          { return true, nil }
func (openRBAC) CanWriteRegistry(uuid.UUID, uuid.UUID) (bool, error)         { return true, nil }
func (openRBAC) IsAdmin(uuid.UUID) (bool, error)                             { return false, nil }
func (openRBAC) GrantProjectAccess(uuid.UUID, uuid.UUID, string) error       { return nil }
func (openRBAC) RevokeProjectAccess(uuid.UUID, uuid.UUID) error              { return nil }
func (openRBAC) MakeAdmin(uuid.UUID) error                                   { return nil }
func (openRBAC) RevokeAdmin(uuid.UUID) error                                 { return nil }
func (openRBAC) GetAllAdminUserIDs() (map[uuid.UUID]bool, error)             { return nil, nil }
func (openRBAC) AddUserToGroup(uuid.UUID, uuid.UUID) error                   { return nil }
func (openRBAC) RemoveUserFromGroup(uuid.UUID, uuid.UUID) error              { return nil }
func (openRBAC) GetUserGroups(uuid.UUID) ([]uuid.UUID, error)                { return nil, nil }
func (openRBAC) GrantGroupProjectAccess(uuid.UUID, uuid.UUID, string) error  { return nil }
func (openRBAC) RevokeGroupProjectAccess(uuid.UUID, uuid.UUID) error         { return nil }
func (openRBAC) GrantGroupRegistryAccess(uuid.UUID, uuid.UUID, string) error { return nil }
func (openRBAC) RevokeGroupRegistryAccess(uuid.UUID, uuid.UUID) error        { return nil }
func (openRBAC) MakeGroupAdmin(uuid.UUID) error                              { return nil }
func (openRBAC) RevokeGroupAdmin(uuid.UUID) error                            { return nil }
func (openRBAC) RemoveAllGroupPolicies(uuid.UUID) error                      { return nil }

func sqliteDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)+"?_pragma=busy_timeout(10000)&_pragma=foreign_keys(ON)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := gdb.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb
}

// stubTeamServer serves the #649 team endpoints with one user, token "tok".
func stubTeamServer(t *testing.T) string {
	t.Helper()
	gdb := sqliteDB(t, "server.db")
	if err := db.Migrate(gdb, false); err != nil {
		t.Fatal(err)
	}
	user := &models.User{ID: uuid.New(), Username: "sam", Email: "sam@example.com"}
	gdb.Create(user)
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/version", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"mode": "team"}) })
	g := v1.Group("")
	g.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer tok" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(auth.UserContextKey, user)
	})
	g.GET("/auth/me", func(c *gin.Context) { c.JSON(http.StatusOK, user) })
	serverapi.Register(g, serverapi.Deps{Core: core.New(core.Deps{DB: gdb, RBAC: openRBAC{}}), RBAC: openRBAC{}})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv.URL
}

func clientRouter(t *testing.T) (http.Handler, *client.Stack, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb := sqliteDB(t, "client.db")
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	stack, err := client.NewStack(client.StackOptions{
		DB: gdb, DataDir: dir, ProjectsDir: filepath.Join(dir, "projects"),
		InlineLog: io.Discard, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	stack.API().Register(r.Group("/api/v1"))
	return r, stack, gdb
}

func call(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestRemoteRoutes(t *testing.T) {
	serverURL := stubTeamServer(t)
	h, stack, gdb := clientRouter(t)
	C := "/api/v1/client"

	rec, body := call(t, h, http.MethodGet, C+"/remote/server", "")
	if rec.Code != http.StatusOK || body["connected"] != false {
		t.Fatalf("GET server: %d %v", rec.Code, body)
	}
	rec, body = call(t, h, http.MethodGet, C+"/remote/projects", "")
	if rec.Code != http.StatusConflict || body["code"] != "not_connected" {
		t.Fatalf("projects while disconnected: %d %v", rec.Code, body)
	}
	rec, body = call(t, h, http.MethodPut, C+"/remote/server", `{"url":"","auth":"none"}`)
	if rec.Code != http.StatusUnprocessableEntity || body["field"] != "url" || body["error"] != "Enter the server’s URL." {
		t.Fatalf("empty url: %d %v", rec.Code, body)
	}
	rec, body = call(t, h, http.MethodPut, C+"/remote/server", `{"url":"`+serverURL+`","auth":"token","token":"tok"}`)
	if rec.Code != http.StatusOK || body["connected"] != true || body["username"] != "sam" {
		t.Fatalf("connect: %d %v", rec.Code, body)
	}

	// A local project with one version, pushed through the API.
	p := models.Project{Name: "demo", OwnerID: stack.SDK.Deps().LocalUserID, Status: models.ProjectStatusReady, Source: "local", Path: t.TempDir()}
	gdb.Create(&p)
	gdb.Create(&models.ProjectVersion{ProjectID: p.ID, VersionNumber: 1, ManifestContent: "a", LockFileContent: "b", PackageMetadata: "[]", CreatedBy: stack.SDK.Deps().LocalUserID})
	gdb.Create(&models.ProjectVersion{ProjectID: p.ID, VersionNumber: 2, ManifestContent: "c", LockFileContent: "d", PackageMetadata: "[]", CreatedBy: stack.SDK.Deps().LocalUserID})

	rec, body = call(t, h, http.MethodPost, C+"/projects/"+p.ID.String()+"/push", `{"up_to":1}`)
	if rec.Code != http.StatusOK || body["server_latest_version"] != float64(1) {
		t.Fatalf("push up_to: %d %v", rec.Code, body)
	}
	rec, body = call(t, h, http.MethodPost, C+"/projects/"+p.ID.String()+"/push", "")
	if rec.Code != http.StatusOK || body["server_latest_version"] != float64(2) {
		t.Fatalf("push: %d %v", rec.Code, body)
	}
	serverID, _ := body["server_project_id"].(string)

	rec, body = call(t, h, http.MethodPost, C+"/remote/projects/refresh", "")
	if rec.Code != http.StatusOK || body["last_synced_at"] == nil {
		t.Fatalf("refresh: %d %v", rec.Code, body)
	}
	projects, _ := body["projects"].([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["local_project_id"] != p.ID.String() {
		t.Fatalf("server projects: %v", body)
	}

	rec, body = call(t, h, http.MethodGet, C+"/remote/projects/"+serverID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remote details: %d %v", rec.Code, body)
	}

	// The project is already here, so pulling it again is refused; a
	// different server project with a taken name gets name_taken.
	rec, body = call(t, h, http.MethodPost, C+"/remote/projects/"+serverID+"/pull", `{"local_name":"other","install":false}`)
	if rec.Code != http.StatusConflict || body["code"] != "already_pulled" {
		t.Fatalf("already pulled: %d %v", rec.Code, body)
	}
	gdb.Where("project_id = ?", p.ID).Delete(&models.ProjectRemote{})
	rec, body = call(t, h, http.MethodPost, C+"/remote/projects/"+serverID+"/pull", `{"local_name":"DEMO","install":false}`)
	if rec.Code != http.StatusConflict || body["code"] != "name_taken" ||
		body["error"] != "A project called DEMO is already on this machine. Pick another name." {
		t.Fatalf("name taken: %d %v", rec.Code, body)
	}
	rec, body = call(t, h, http.MethodPost, C+"/remote/projects/"+serverID+"/pull", `{"local_name":"demo-copy","install":false}`)
	if rec.Code != http.StatusAccepted || body["project_id"] == nil {
		t.Fatalf("pull: %d %v", rec.Code, body)
	}
	job, _ := body["job"].(map[string]any)
	if job["type"] != "pull" || job["status"] != "completed" || job["version_number"] != float64(2) {
		t.Fatalf("pull job: %v", job)
	}
	rec, _ = call(t, h, http.MethodDelete, C+"/remote/server", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect: %d", rec.Code)
	}
	rec, body = call(t, h, http.MethodPost, C+"/projects/"+p.ID.String()+"/versions/2/pull", "")
	if rec.Code != http.StatusConflict || body["code"] != "not_connected" {
		t.Fatalf("pull version while disconnected: %d %v", rec.Code, body)
	}
}
