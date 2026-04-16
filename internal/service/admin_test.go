package service

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func adminTestSetup(t *testing.T) (*AdminService, *WorkspaceService, *gorm.DB) {
	t.Helper()
	wsSvc, db := testSetup(t, false)
	return NewAdminService(db, rbac.NewDefaultProvider()), wsSvc, db
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

// --- CreateUser ---

func TestAdminCreateUser(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	user, err := svc.CreateUser(CreateUserRequest{
		Username: "newuser",
		Email:    "new@test.com",
		Password: "securepassword",
	}, adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "newuser" {
		t.Errorf("expected username 'newuser', got %q", user.Username)
	}
	if user.PasswordHash == "" {
		t.Error("expected password to be hashed")
	}
	if user.PasswordHash == "securepassword" {
		t.Error("password should be hashed, not stored in plaintext")
	}

	// Verify audit log
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", adminID, "create_user").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestAdminCreateUser_WithAdmin(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	user, err := svc.CreateUser(CreateUserRequest{
		Username: "newadmin",
		Email:    "admin2@test.com",
		Password: "password",
		IsAdmin:  true,
	}, adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the user got admin status via GetUser
	result, err := svc.GetUser(user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !result.IsAdmin {
		t.Error("expected user to be admin")
	}
}

// --- GetUser ---

func TestAdminGetUser_NotFound(t *testing.T) {
	svc, _, _ := adminTestSetup(t)

	_, err := svc.GetUser(uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ToggleAdmin ---

func TestAdminToggleAdmin(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	userID := createTestUser(t, db, "user")

	// Make admin
	result, err := svc.ToggleAdmin(userID, adminID)
	if err != nil {
		t.Fatalf("toggle on: %v", err)
	}
	if !result.IsAdmin {
		t.Error("expected IsAdmin=true after toggle on")
	}

	// Revoke admin
	result, err = svc.ToggleAdmin(userID, adminID)
	if err != nil {
		t.Fatalf("toggle off: %v", err)
	}
	if result.IsAdmin {
		t.Error("expected IsAdmin=false after toggle off")
	}
}

func TestAdminToggleAdmin_NotFound(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	_, err := svc.ToggleAdmin(uuid.New(), adminID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- DeleteUser ---

func TestAdminDeleteUser(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	userID := createTestUser(t, db, "victim")

	if err := svc.DeleteUser(userID, adminID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify deleted
	var count int64
	db.Model(&models.User{}).Where("id = ?", userID).Count(&count)
	if count != 0 {
		t.Error("expected user to be deleted")
	}
}

func TestAdminDeleteUser_CannotDeleteSelf(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	err := svc.DeleteUser(adminID, adminID)
	if err == nil {
		t.Fatal("expected error for self-deletion")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestAdminDeleteUser_NotFound(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")

	err := svc.DeleteUser(uuid.New(), adminID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListRoles ---

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
	svc, wsSvc, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	userID := createTestUser(t, db, "user")
	ws := createReadyWorkspace(t, wsSvc, db, "test-ws", adminID)
	db.Create(&models.Role{Name: "editor"})

	var role models.Role
	db.Where("name = ?", "editor").First(&role)

	perm, err := svc.GrantPermission(userID, ws.ID, role.ID, adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, perm.UserID)
	}
}

func TestAdminGrantPermission_UserNotFound(t *testing.T) {
	svc, wsSvc, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	ws := createReadyWorkspace(t, wsSvc, db, "test-ws", adminID)

	_, err := svc.GrantPermission(uuid.New(), ws.ID, 1, adminID)
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

// --- RevokePermission ---

func TestAdminRevokePermission(t *testing.T) {
	svc, wsSvc, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	userID := createTestUser(t, db, "user")
	ws := createReadyWorkspace(t, wsSvc, db, "test-ws", adminID)
	db.Create(&models.Role{Name: "viewer"})

	var role models.Role
	db.Where("name = ?", "viewer").First(&role)

	perm, _ := svc.GrantPermission(userID, ws.ID, role.ID, adminID)

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

	// Create a user to generate audit log
	svc.CreateUser(CreateUserRequest{
		Username: "auditme",
		Email:    "audit@test.com",
		Password: "password",
	}, adminID)

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

	svc.CreateUser(CreateUserRequest{
		Username: "u1", Email: "u1@test.com", Password: "pass",
	}, adminID)

	logs, err := svc.ListAuditLogs("", "create_user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 filtered audit log, got %d", len(logs))
	}
}

// --- GetDashboardStats ---

func TestAdminGetDashboardStats(t *testing.T) {
	svc, _, _ := adminTestSetup(t)

	stats, err := svc.GetDashboardStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalDiskUsageBytes != 0 {
		t.Errorf("expected 0 bytes with no workspaces, got %d", stats.TotalDiskUsageBytes)
	}
}
