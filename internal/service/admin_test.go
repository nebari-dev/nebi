package service

import (
	"errors"
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

// --- Federated identity reviews ---

func TestAdminListFederatedIdentityReviews(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	localUser := models.User{
		Username:     "local-alice",
		Email:        "alice@test.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}
	review := models.FederatedIdentityReview{
		UserID:         localUser.ID,
		Issuer:         "https://issuer.example.com",
		Subject:        "sub-alice",
		CollisionField: "email",
		Username:       "alice",
		Email:          "alice@test.com",
		EmailVerified:  true,
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	reviews, err := svc.ListFederatedIdentityReviews()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].User.ID != localUser.ID {
		t.Errorf("expected review user to be preloaded, got %s", reviews[0].User.ID)
	}
}

func TestAdminApproveFederatedIdentityReview(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	localUser := models.User{
		Username:     "local-bob",
		Email:        "bob@test.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}
	review := models.FederatedIdentityReview{
		UserID:         localUser.ID,
		Issuer:         "https://issuer.example.com",
		Subject:        "sub-bob",
		CollisionField: "username",
		Username:       "local-bob",
		Email:          "bob@test.com",
		EmailVerified:  true,
		Name:           "Bob",
		AvatarURL:      "https://example.com/bob.png",
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	identity, err := svc.ApproveFederatedIdentityReview(review.ID, adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.UserID != localUser.ID {
		t.Errorf("expected identity user %s, got %s", localUser.ID, identity.UserID)
	}
	if identity.Issuer != review.Issuer || identity.Subject != review.Subject {
		t.Errorf("expected issuer/subject %s/%s, got %s/%s", review.Issuer, review.Subject, identity.Issuer, identity.Subject)
	}

	var reviewCount int64
	db.Model(&models.FederatedIdentityReview{}).Where("id = ?", review.ID).Count(&reviewCount)
	if reviewCount != 0 {
		t.Errorf("expected review to be deleted, got %d", reviewCount)
	}

	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", adminID, "approve_federated_identity").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestAdminRejectFederatedIdentityReview(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	localUser := models.User{
		Username:     "local-carol",
		Email:        "carol@test.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}
	review := models.FederatedIdentityReview{
		UserID:         localUser.ID,
		Issuer:         "https://issuer.example.com",
		Subject:        "sub-carol",
		CollisionField: "email",
		Username:       "carol",
		Email:          "carol@test.com",
		EmailVerified:  true,
		Status:         models.FederatedIdentityReviewStatusPending,
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	if err := svc.RejectFederatedIdentityReview(review.ID, adminID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rejected models.FederatedIdentityReview
	if err := db.First(&rejected, "id = ?", review.ID).Error; err != nil {
		t.Fatalf("expected review to remain as rejected: %v", err)
	}
	if rejected.Status != models.FederatedIdentityReviewStatusRejected {
		t.Errorf("expected rejected status, got %q", rejected.Status)
	}
	if rejected.ReviewedBy == nil || *rejected.ReviewedBy != adminID {
		t.Errorf("expected reviewed_by %s, got %v", adminID, rejected.ReviewedBy)
	}
	if rejected.ReviewedAt == nil {
		t.Error("expected reviewed_at to be set")
	}

	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", adminID, "reject_federated_identity").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestAdminApproveRejectedFederatedIdentityReviewConflicts(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	adminID := createTestUser(t, db, "admin")
	localUser := models.User{
		Username:     "local-dana",
		Email:        "dana@test.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}
	review := models.FederatedIdentityReview{
		UserID:         localUser.ID,
		Issuer:         "https://issuer.example.com",
		Subject:        "sub-dana",
		CollisionField: "email",
		Username:       "dana",
		Email:          "dana@test.com",
		EmailVerified:  true,
		Status:         models.FederatedIdentityReviewStatusRejected,
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	identity, err := svc.ApproveFederatedIdentityReview(review.ID, adminID)
	if err == nil {
		t.Fatalf("expected conflict, got identity=%v", identity)
	}
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
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

// --- Group admin & registry grants ---

func TestGrantGroupAdmin_MembersBecomeEffectiveAdmins(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "admins"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)

	if err := svc.GrantGroupAdmin(g.ID, admin); err != nil {
		t.Fatalf("grant group admin: %v", err)
	}

	provider := rbac.NewDefaultProvider()
	isAdmin, err := provider.IsAdmin(alice)
	if err != nil || !isAdmin {
		t.Fatalf("alice should be admin via group, err=%v admin=%v", err, isAdmin)
	}
}

func TestRevokeGroupAdmin_RemovesEffectiveAdmin(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "admins"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)
	_ = svc.GrantGroupAdmin(g.ID, admin)

	if err := svc.RevokeGroupAdmin(g.ID, admin); err != nil {
		t.Fatalf("revoke group admin: %v", err)
	}

	isAdmin, _ := rbac.NewDefaultProvider().IsAdmin(alice)
	if isAdmin {
		t.Fatalf("alice should no longer be admin")
	}
}

func TestGrantRegistryToGroup_GivesTransitiveAccess(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "reg-team"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)

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
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)

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
