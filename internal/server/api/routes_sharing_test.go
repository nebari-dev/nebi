package api_test

// Team-mode HTTP tests for the sharing routes, through the production
// router (real login, auth middleware and casbin RBAC).

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/api"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/server/core"
	"gorm.io/gorm"
)

const testPassword = "password"

type teamServer struct {
	t       *testing.T
	router  http.Handler
	db      *gorm.DB
	project uuid.UUID
	users   map[string]models.User
	tokens  map[string]string
}

func newTeamServer(t *testing.T) *teamServer {
	t.Helper()
	cfg := &config.Config{Mode: config.ModeTeam}
	cfg.Auth.Type = "basic"
	cfg.Auth.JWTSecret = "test-secret-for-sharing-routes"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "sharing-routes.db")
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
	q := queue.NewMemoryQueue(16)
	t.Cleanup(func() { q.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := &teamServer{
		t:      t,
		router: api.NewRouter(cfg, database, q, exec, nil, logger),
		db:     database,
		users:  map[string]models.User{},
		tokens: map[string]string{},
	}
	for _, name := range []string{"owner", "editor", "stranger", "admin"} {
		ts.addUser(name)
	}
	if err := rbac.NewDefaultProvider().MakeAdmin(ts.users["admin"].ID); err != nil {
		t.Fatal(err)
	}
	p := models.Project{Name: "ml-baseline", OwnerID: ts.users["owner"].ID, Status: models.ProjectStatusReady}
	if err := database.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := rbac.NewDefaultProvider().GrantProjectAccess(ts.users["owner"].ID, p.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	ts.project = p.ID
	return ts
}

func (ts *teamServer) addUser(name string) {
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		ts.t.Fatal(err)
	}
	u := models.User{Username: name, Email: name + "@example.com", PasswordHash: hash}
	if err := ts.db.Create(&u).Error; err != nil {
		ts.t.Fatal(err)
	}
	ts.users[name] = u
	rec := ts.do("", http.MethodPost, "/api/v1/auth/login", `{"username":"`+name+`","password":"`+testPassword+`"}`)
	var resp struct {
		Token string `json:"token"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || resp.Token == "" {
		ts.t.Fatalf("login %s: %d %s", name, rec.Code, rec.Body)
	}
	ts.tokens[name] = resp.Token
}

func (ts *teamServer) do(as, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if as != "" {
		req.Header.Set("Authorization", "Bearer "+ts.tokens[as])
	}
	rec := httptest.NewRecorder()
	ts.router.ServeHTTP(rec, req)
	return rec
}

func (ts *teamServer) expect(as, method, path, body string, want int) *httptest.ResponseRecorder {
	ts.t.Helper()
	rec := ts.do(as, method, path, body)
	if rec.Code != want {
		ts.t.Fatalf("%s %s as %q: want %d, got %d: %s", method, path, as, want, rec.Code, rec.Body)
	}
	return rec
}

func decodeAccess(t *testing.T, rec *httptest.ResponseRecorder) core.AccessList {
	t.Helper()
	var list core.AccessList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body)
	}
	return list
}

func TestSharingRoutesEnforceRBAC(t *testing.T) {
	ts := newTeamServer(t)
	access := "/api/v1/projects/" + ts.project.String() + "/access"
	editorID := ts.users["editor"].ID.String()
	strangerID := ts.users["stranger"].ID.String()

	// Unauthenticated callers never reach the routes.
	ts.expect("", http.MethodGet, access, "", http.StatusUnauthorized)
	ts.expect("", http.MethodGet, "/api/v1/principals?q=ed", "", http.StatusUnauthorized)

	// Strangers can neither read nor manage access.
	ts.expect("stranger", http.MethodGet, access, "", http.StatusForbidden)
	ts.expect("stranger", http.MethodPost, access, `{"principals":[{"kind":"user","id":"`+strangerID+`"}],"role":"edit"}`, http.StatusForbidden)

	// The owner shares with the editor.
	list := decodeAccess(t, ts.expect("owner", http.MethodPost, access, `{"principals":[{"kind":"user","id":"`+editorID+`"}],"role":"edit"}`, http.StatusOK))
	if len(list.Entries) != 2 || list.Entries[1].Role != "edit" || !list.CanManage {
		t.Fatalf("after grant: %+v", list)
	}

	// The editor now reads the project and its access list, but cannot
	// manage access or delete the project from the server.
	ts.expect("editor", http.MethodGet, "/api/v1/projects/"+ts.project.String(), "", http.StatusOK)
	list = decodeAccess(t, ts.expect("editor", http.MethodGet, access, "", http.StatusOK))
	if list.CanManage {
		t.Fatal("editor must not manage access")
	}
	ts.expect("editor", http.MethodPost, access, `{"principals":[{"kind":"user","id":"`+strangerID+`"}],"role":"view"}`, http.StatusForbidden)
	ts.expect("editor", http.MethodPut, access+"/user/"+editorID, `{"role":"view"}`, http.StatusForbidden)
	ts.expect("editor", http.MethodDelete, access+"/user/"+editorID, "", http.StatusForbidden)
	ts.expect("editor", http.MethodDelete, "/api/v1/catalog/projects/"+ts.project.String(), "", http.StatusForbidden)

	// Role change and removal by the owner.
	list = decodeAccess(t, ts.expect("owner", http.MethodPut, access+"/user/"+editorID, `{"role":"view"}`, http.StatusOK))
	if list.Entries[1].Role != "view" {
		t.Fatalf("after update: %+v", list)
	}
	ts.expect("editor", http.MethodPut, "/api/v1/projects/"+ts.project.String()+"/pixi-toml", `{"content":"x"}`, http.StatusForbidden)
	ts.expect("owner", http.MethodDelete, access+"/user/"+editorID, "", http.StatusOK)
	ts.expect("editor", http.MethodGet, access, "", http.StatusForbidden)

	// The owner cannot be removed; bad input is rejected.
	rec := ts.expect("owner", http.MethodDelete, access+"/user/"+ts.users["owner"].ID.String(), "", http.StatusConflict)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"last_owner"`)) {
		t.Fatalf("want last_owner code: %s", rec.Body)
	}
	ts.expect("owner", http.MethodPost, access, `{"principals":[{"kind":"user","id":"`+editorID+`"}],"role":"owner"}`, http.StatusUnprocessableEntity)
	ts.expect("owner", http.MethodGet, "/api/v1/projects/not-a-uuid/access", "", http.StatusBadRequest)

	// Admins manage projects they do not own.
	list = decodeAccess(t, ts.expect("admin", http.MethodGet, access, "", http.StatusOK))
	if !list.CanManage {
		t.Fatal("admin must manage access")
	}

	// Search is available to any signed-in user.
	var principals []core.Principal
	rec = ts.expect("stranger", http.MethodGet, "/api/v1/principals?q=edit", "", http.StatusOK)
	if err := json.Unmarshal(rec.Body.Bytes(), &principals); err != nil || len(principals) != 1 || principals[0].ID != editorID {
		t.Fatalf("principals: %v %s", err, rec.Body)
	}

	// Delete from the server: owner only (or admin), runs as a job.
	ts.expect("owner", http.MethodDelete, "/api/v1/catalog/projects/"+ts.project.String(), "", http.StatusAccepted)
	var jobs int64
	ts.db.Model(&models.Job{}).Where("project_id = ? AND type = ?", ts.project, models.JobTypeDelete).Count(&jobs)
	if jobs != 1 {
		t.Fatalf("want one delete job, got %d", jobs)
	}
}
