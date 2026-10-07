package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupGroupTestRouter(t *testing.T) (*gin.Engine, *gorm.DB, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Role{}, &models.Group{},
		&models.GroupMember{}, &models.GroupPermission{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("rbac: %v", err)
	}

	h := NewGroupHandler(service.NewGroupService(db))

	user := models.User{Username: "admin", Email: "admin@test"}
	db.Create(&user)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &user)
		c.Next()
	})
	admin := r.Group("/api/v1/admin")
	{
		admin.GET("/groups", h.ListGroups)
		admin.GET("/groups/:id", h.GetGroup)
		admin.GET("/groups/:id/members", h.ListMembers)
	}
	r.GET("/api/v1/groups/me", h.MyGroups)
	return r, db, user.ID
}

func TestGroupHandlers_ReadIdentityProviderGroups(t *testing.T) {
	r, db, callerID := setupGroupTestRouter(t)
	g := models.Group{Name: "synced"}
	db.Create(&g)
	db.Create(&models.GroupMember{GroupID: g.ID, UserID: callerID})

	get := func(path string, out any) {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d body=%s", path, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("GET %s: decode: %v", path, err)
		}
	}

	var list []service.GroupWithMemberCount
	get("/api/v1/admin/groups", &list)
	if len(list) != 1 || list[0].Name != "synced" || list[0].MemberCount != 1 {
		t.Fatalf("unexpected group list %+v", list)
	}
	var one service.GroupWithMemberCount
	get("/api/v1/admin/groups/"+g.ID.String(), &one)
	if one.ID != g.ID {
		t.Fatalf("unexpected group %+v", one)
	}
	var members []models.GroupMember
	get("/api/v1/admin/groups/"+g.ID.String()+"/members", &members)
	if len(members) != 1 || members[0].UserID != callerID {
		t.Fatalf("unexpected members %+v", members)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/"+uuid.NewString(), nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown group, got %d", w.Code)
	}
}

func TestMyGroups_ReturnsOnlyCallersGroups(t *testing.T) {
	r, db, callerID := setupGroupTestRouter(t)

	mine := models.Group{Name: "mine"}
	db.Create(&mine)
	db.Create(&models.GroupMember{GroupID: mine.ID, UserID: callerID})
	db.Create(&models.Group{Name: "theirs"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []models.Group
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Name != "mine" {
		t.Fatalf("expected single 'mine' group, got %+v", out)
	}
}
