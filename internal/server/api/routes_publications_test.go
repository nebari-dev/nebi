package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/server/core"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeRBAC grants write only to the listed users.
type fakeRBAC struct {
	rbac.Provider
	writers map[uuid.UUID]bool
}

func (f fakeRBAC) CanReadProject(userID, _ uuid.UUID) (bool, error)  { return true, nil }
func (f fakeRBAC) CanWriteProject(userID, _ uuid.UUID) (bool, error) { return f.writers[userID], nil }

func TestPublicationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "api.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb, false); err != nil {
		t.Fatal(err)
	}
	owner := models.User{Username: "owner", Email: "owner@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	gdb.Create(&owner)
	gdb.Create(&viewer)
	p := models.Project{Name: "p", OwnerID: owner.ID, Status: models.ProjectStatusReady}
	gdb.Omit("Owner").Create(&p)
	gdb.Omit("Project", "Job", "CreatedByUser").Create(&models.ProjectVersion{ProjectID: p.ID, VersionNumber: 1, ManifestContent: "x", LockFileContent: "y", PackageMetadata: "[]", CreatedBy: owner.ID})

	svc := core.New(core.Deps{DB: gdb})
	newRouter := func(u *models.User) *gin.Engine {
		r := gin.New()
		g := r.Group("/api/v1", func(c *gin.Context) { c.Set(auth.UserContextKey, u); c.Next() })
		Register(g, Deps{Core: svc, RBAC: fakeRBAC{writers: map[uuid.UUID]bool{owner.ID: true}}})
		return r
	}
	do := func(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	base := "/api/v1/projects/" + p.ID.String() + "/sync/versions/1/publications"
	body := `{"registry_host":"ghcr.io","repository":"org/p","tag":"1","digest":"sha256:abc"}`

	if rec := do(newRouter(&viewer), http.MethodPost, base, body); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer record: want 403, got %d %s", rec.Code, rec.Body)
	}
	rec := do(newRouter(&owner), http.MethodPost, base, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("record: %d %s", rec.Code, rec.Body)
	}
	var out core.PublicationRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == nil {
		t.Fatalf("decode: %v %s", err, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("publication response mentions credentials")
	}
	if rec := do(newRouter(&owner), http.MethodPost, "/api/v1/projects/"+p.ID.String()+"/sync/versions/7/publications", body); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown version: want 404, got %d", rec.Code)
	}
	if rec := do(newRouter(&owner), http.MethodPost, base, `{"repository":"org/p","tag":"1"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing host: want 422, got %d", rec.Code)
	}
	if rec := do(newRouter(&viewer), http.MethodDelete, base+"/"+out.ID.String(), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer delete: want 403, got %d", rec.Code)
	}
	if rec := do(newRouter(&owner), http.MethodDelete, base+"/"+out.ID.String(), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := do(newRouter(&owner), http.MethodDelete, base+"/"+uuid.NewString(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete unknown: want 404, got %d", rec.Code)
	}
}
