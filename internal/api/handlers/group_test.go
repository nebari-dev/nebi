package handlers

import (
	"bytes"
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

	groupSvc := service.NewGroupService(db, rbac.NewDefaultProvider())
	h := NewGroupHandler(groupSvc)

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
		admin.POST("/groups", h.CreateGroup)
		admin.GET("/groups", h.ListGroups)
		admin.GET("/groups/:id", h.GetGroup)
		admin.PATCH("/groups/:id", h.UpdateGroup)
		admin.DELETE("/groups/:id", h.DeleteGroup)
		admin.POST("/groups/:id/members", h.AddMember)
		admin.DELETE("/groups/:id/members/:user_id", h.RemoveMember)
	}
	r.GET("/api/v1/groups/me", h.MyGroups)
	return r, db, user.ID
}

func TestCreateGroup_Handler201(t *testing.T) {
	r, _, _ := setupGroupTestRouter(t)
	body, _ := json.Marshal(map[string]string{"name": "team-a"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	var out models.Group
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Name != "team-a" {
		t.Errorf("expected name 'team-a', got %q", out.Name)
	}
}

func TestPatchGroup_OIDCReturns409(t *testing.T) {
	r, db, _ := setupGroupTestRouter(t)
	g := models.Group{Name: "synced", Source: models.GroupSourceOIDC}
	db.Create(&g)

	body, _ := json.Marshal(map[string]string{"description": "x"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/groups/"+g.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestMyGroups_ReturnsOnlyCallersGroups(t *testing.T) {
	r, db, callerID := setupGroupTestRouter(t)
	groupSvc := service.NewGroupService(db, rbac.NewDefaultProvider())

	mine, _ := groupSvc.CreateGroup(service.CreateGroupRequest{Name: "mine"}, callerID)
	_ = groupSvc.AddMember(mine.ID, callerID, callerID)
	_, _ = groupSvc.CreateGroup(service.CreateGroupRequest{Name: "theirs"}, callerID)

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
