package service

import (
	"errors"
	"testing"

	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func groupTestSetup(t *testing.T) (*GroupService, *gorm.DB) {
	t.Helper()
	_, db := testSetup(t, false)
	return NewGroupService(db, rbac.NewDefaultProvider()), db
}

func TestCreateGroup_NativeSucceeds(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")

	g, err := svc.CreateGroup(CreateGroupRequest{Name: "data-science", Description: "DS team"}, admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Name != "data-science" {
		t.Errorf("expected name 'data-science', got %q", g.Name)
	}
	if g.Source != models.GroupSourceNative {
		t.Errorf("expected source native, got %q", g.Source)
	}

	var count int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", admin, "create_group").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log, got %d", count)
	}
}

func TestCreateGroup_DuplicateNameFails(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	_, _ = svc.CreateGroup(CreateGroupRequest{Name: "dup"}, admin)

	_, err := svc.CreateGroup(CreateGroupRequest{Name: "dup"}, admin)
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

func TestAddGroupMember_GrantsCasbinMembership(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := svc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)

	if err := svc.AddMember(g.ID, alice, admin); err != nil {
		t.Fatalf("add member: %v", err)
	}

	groups, err := rbac.GetUserGroups(alice)
	if err != nil {
		t.Fatalf("get user groups: %v", err)
	}
	found := false
	for _, gid := range groups {
		if gid == g.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected alice to be in group %s, got %v", g.ID, groups)
	}
}

func TestRemoveGroupMember_RevokesCasbinMembership(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := svc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)
	_ = svc.AddMember(g.ID, alice, admin)

	if err := svc.RemoveMember(g.ID, alice, admin); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	groups, _ := rbac.GetUserGroups(alice)
	for _, gid := range groups {
		if gid == g.ID {
			t.Fatalf("expected alice removed from group, still present")
		}
	}
}

func TestDeleteGroup_HardRemovesCasbinRules(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := svc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)
	_ = svc.AddMember(g.ID, alice, admin)

	if err := svc.DeleteGroup(g.ID, admin); err != nil {
		t.Fatalf("delete group: %v", err)
	}

	groups, _ := rbac.GetUserGroups(alice)
	if len(groups) != 0 {
		t.Fatalf("expected casbin membership removed, got %v", groups)
	}

	// Group must be hard-deleted so its name is freed for re-creation
	// (the unique index on name does not honour gorm.DeletedAt).
	var dbGroup models.Group
	err := db.Unscoped().First(&dbGroup, "id = ?", g.ID).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected group row hard-deleted, got err=%v dbGroup=%+v", err, dbGroup)
	}
}

func TestDeleteGroup_FreesNameForRecreate(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")

	first, err := svc.CreateGroup(CreateGroupRequest{Name: "engineers"}, admin)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := svc.DeleteGroup(first.ID, admin); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Recreating with the same name must not fail with a unique-constraint
	// violation from a lingering soft-deleted row. This is the scenario the
	// OIDC sync hits when an admin deletes a native group that an IdP claim
	// later re-introduces.
	second, err := svc.CreateGroup(CreateGroupRequest{Name: "engineers"}, admin)
	if err != nil {
		t.Fatalf("recreate after delete: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("expected fresh group row (new ID), got same ID %s", first.ID)
	}
	_ = db
}

func TestDeleteGroup_OIDCRejected(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	g := models.Group{Name: "synced", Source: models.GroupSourceOIDC}
	if err := db.Create(&g).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}

	err := svc.DeleteGroup(g.ID, admin)
	if err == nil {
		t.Fatal("expected ConflictError for OIDC group, got nil")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	var auditCount int64
	db.Model(&models.AuditLog{}).Where("action = ?", "delete_group").Count(&auditCount)
	if auditCount != 0 {
		t.Errorf("expected no audit log for rejected OIDC delete, got %d", auditCount)
	}
}

func TestUpdateGroup_OIDCRejected(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	g := models.Group{Name: "synced", Source: models.GroupSourceOIDC}
	if err := db.Create(&g).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}

	_, err := svc.UpdateGroup(g.ID, UpdateGroupRequest{Description: ptr("new")}, admin)
	if err == nil {
		t.Fatal("expected ConflictError for OIDC group, got nil")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	var auditCount int64
	db.Model(&models.AuditLog{}).Where("action = ?", "update_group").Count(&auditCount)
	if auditCount != 0 {
		t.Errorf("expected no audit log for rejected OIDC update, got %d", auditCount)
	}
}

func ptr[T any](v T) *T { return &v }
