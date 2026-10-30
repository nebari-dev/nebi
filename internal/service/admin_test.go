package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func adminTestSetup(t *testing.T) (*AdminService, *ProjectService, *gorm.DB) {
	t.Helper()
	projectSvc, db := testSetup(t, false)
	return NewAdminService(db, rbac.NewDefaultProvider(), limits.Defaults()), projectSvc, db
}

// --- ListUsers ---

func TestAdminListUsers(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	createTestUser(t, db, "alice")
	createTestUser(t, db, "bob")

	users, err := svc.ListUsers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestAdminGetUser_NotFound(t *testing.T) {
	svc, _, _ := adminTestSetup(t)

	_, err := svc.GetUser(uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAdminListRoles(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	db.Create(&models.Role{Name: "viewer"})
	db.Create(&models.Role{Name: "editor"})

	roles, err := svc.ListRoles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

// --- GrantPermission ---

func TestAdminGrantPermission(t *testing.T) {
	svc, projectSvc, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	userID := createTestUser(t, db, "user")
	project := createReadyProject(t, projectSvc, db, "test-ws", adminID)
	db.Create(&models.Role{Name: "editor"})

	var role models.Role
	db.Where("name = ?", "editor").First(&role)

	perm, err := svc.GrantPermission(userID, project.ID, role.ID, adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, perm.UserID)
	}
}

func TestAdminGrantPermission_UserNotFound(t *testing.T) {
	svc, projectSvc, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	project := createReadyProject(t, projectSvc, db, "test-ws", adminID)

	_, err := svc.GrantPermission(uuid.New(), project.ID, 1, adminID)
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

// --- RevokePermission ---

func TestAdminRevokePermission(t *testing.T) {
	svc, projectSvc, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	userID := createTestUser(t, db, "user")
	project := createReadyProject(t, projectSvc, db, "test-ws", adminID)
	db.Create(&models.Role{Name: "viewer"})

	var role models.Role
	db.Where("name = ?", "viewer").First(&role)

	perm, _ := svc.GrantPermission(userID, project.ID, role.ID, adminID)

	err := svc.RevokePermission(fmt.Sprintf("%d", perm.ID), adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify deleted
	var count int64
	db.Model(&models.Permission{}).Where("id = ?", perm.ID).Count(&count)
	if count != 0 {
		t.Error("expected permission to be deleted")
	}
}

func TestAdminRevokePermission_NotFound(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	err := svc.RevokePermission("99999", adminID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListAuditLogs ---

func TestAdminListAuditLogs(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	audit.LogAction(db, adminID, audit.ActionCreateGroup, "group:x", nil)

	logs, err := svc.ListAuditLogs("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) == 0 {
		t.Error("expected at least 1 audit log")
	}
}

func TestAdminListAuditLogs_FilterByAction(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	audit.LogAction(db, adminID, audit.ActionCreateGroup, "group:x", nil)
	audit.LogAction(db, adminID, audit.ActionAddGroupMember, "group:x", nil)

	logs, err := svc.ListAuditLogs("", audit.ActionAddGroupMember)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 filtered audit log, got %d", len(logs))
	}
}

func TestAdminGetDashboardStats(t *testing.T) {
	svc, _, _ := adminTestSetup(t)

	stats, err := svc.GetDashboardStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalDiskUsageBytes != 0 {
		t.Errorf("expected 0 bytes with no projects, got %d", stats.TotalDiskUsageBytes)
	}
}

func TestAdminGetResourceMetrics(t *testing.T) {
	svc, projectSvc, db := adminTestSetup(t)
	userID := createTestUser(t, db, "alice")

	if _, err := projectSvc.Create(context.Background(), CreateRequest{Name: "metrics-ws"}, userID); err != nil {
		t.Fatalf("create project: %v", err)
	}

	metrics, err := svc.GetResourceMetrics()
	if err != nil {
		t.Fatalf("GetResourceMetrics: %v", err)
	}
	if metrics.ActiveJobsGlobal != 1 {
		t.Fatalf("expected 1 active job, got %d", metrics.ActiveJobsGlobal)
	}
	if len(metrics.ActiveJobsByUser) != 1 || metrics.ActiveJobsByUser[0].UserID != userID {
		t.Fatalf("expected active job usage for %s, got %+v", userID, metrics.ActiveJobsByUser)
	}
}

func TestAdminGetResourceMetrics_AttributesLegacyJobsToProjectOwner(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	userID := createTestUser(t, db, "alice")
	project := models.Project{
		ID:      uuid.New(),
		Name:    "legacy-metrics-ws",
		OwnerID: userID,
		Status:  models.ProjectStatusReady,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	zeroUserJob := models.Job{
		ID:        uuid.New(),
		ProjectID: project.ID,
		UserID:    uuid.Nil,
		Type:      models.JobTypeInstall,
		Status:    models.JobStatusPending,
	}
	emptyUserJob := models.Job{
		ID:        uuid.New(),
		ProjectID: project.ID,
		UserID:    uuid.Nil,
		Type:      models.JobTypeRemove,
		Status:    models.JobStatusRunning,
	}
	if err := db.Create(&zeroUserJob).Error; err != nil {
		t.Fatalf("create zero user job: %v", err)
	}
	if err := db.Create(&emptyUserJob).Error; err != nil {
		t.Fatalf("create empty user job: %v", err)
	}
	if err := db.Model(&models.Job{}).Where("id = ?", emptyUserJob.ID).Update("user_id", "").Error; err != nil {
		t.Fatalf("blank legacy user_id: %v", err)
	}

	metrics, err := svc.GetResourceMetrics()
	if err != nil {
		t.Fatalf("GetResourceMetrics: %v", err)
	}
	if metrics.ActiveJobsGlobal != 2 {
		t.Fatalf("expected 2 active jobs, got %d", metrics.ActiveJobsGlobal)
	}
	if len(metrics.ActiveJobsByUser) != 1 {
		t.Fatalf("expected one user bucket, got %+v", metrics.ActiveJobsByUser)
	}
	if metrics.ActiveJobsByUser[0].UserID != userID || metrics.ActiveJobsByUser[0].ActiveJobs != 2 {
		t.Fatalf("expected 2 active jobs attributed to %s, got %+v", userID, metrics.ActiveJobsByUser[0])
	}
}

// --- Registry grants ---

func TestGrantRegistryToGroup_GivesTransitiveAccess(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g := createTestGroup(t, db, "reg-team")
	addTestGroupMember(t, db, g.ID, alice)

	reg := models.OCIRegistry{Name: "private", URL: "ghcr.io", Namespace: "ns"}
	if err := db.Create(&reg).Error; err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	if err := svc.GrantRegistryToGroup(reg.ID, g.ID, "write", admin); err != nil {
		t.Fatalf("grant registry: %v", err)
	}

	can, err := rbac.NewDefaultProvider().CanWriteRegistry(alice, reg.ID)
	if err != nil || !can {
		t.Fatalf("alice should have write on registry, err=%v can=%v", err, can)
	}
}

func TestListUserGroups_ReturnsOnlyTheirs(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")

	g := createTestGroup(t, db, "ds")
	addTestGroupMember(t, db, g.ID, alice)

	aliceGroups, err := svc.ListUserGroups(alice)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(aliceGroups) != 1 || aliceGroups[0].ID != g.ID {
		t.Fatalf("expected alice in 1 group %s, got %+v", g.ID, aliceGroups)
	}

	bobGroups, _ := svc.ListUserGroups(bob)
	if len(bobGroups) != 0 {
		t.Fatalf("expected bob in 0 groups, got %d", len(bobGroups))
	}
}
