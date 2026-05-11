package rbac

import (
	"log/slog"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func TestGroupGrantsWorkspaceAccessTransitively(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	group := uuid.New()
	ws := uuid.New()

	if err := AddUserToGroup(user, group); err != nil {
		t.Fatalf("add user to group: %v", err)
	}
	if err := GrantGroupWorkspaceAccess(group, ws, "viewer"); err != nil {
		t.Fatalf("grant group: %v", err)
	}

	canRead, err := CanReadWorkspace(user, ws)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if !canRead {
		t.Fatalf("expected user to read workspace via group, got false")
	}
}

func TestGroupAdminTransitive(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	group := uuid.New()

	if err := AddUserToGroup(user, group); err != nil {
		t.Fatalf("add user to group: %v", err)
	}
	if err := MakeGroupAdmin(group); err != nil {
		t.Fatalf("make group admin: %v", err)
	}

	isAdmin, err := IsAdmin(user)
	if err != nil {
		t.Fatalf("is admin: %v", err)
	}
	if !isAdmin {
		t.Fatalf("expected user to be admin via group, got false")
	}
}

func TestDirectUserPolicyStillWorksAfterMatcherChange(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	ws := uuid.New()

	if err := GrantWorkspaceAccess(user, ws, "editor"); err != nil {
		t.Fatalf("grant workspace: %v", err)
	}

	canWrite, err := CanWriteWorkspace(user, ws)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if !canWrite {
		t.Fatalf("expected direct write policy to still match, got false")
	}
}

func TestRemoveAllGroupPoliciesCleansEverything(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	group := uuid.New()
	ws := uuid.New()
	reg := uuid.New()

	_ = AddUserToGroup(user, group)
	_ = GrantGroupWorkspaceAccess(group, ws, "editor")
	_ = GrantGroupRegistryAccess(group, reg, "write")
	_ = MakeGroupAdmin(group)

	if err := RemoveAllGroupPolicies(group); err != nil {
		t.Fatalf("remove all: %v", err)
	}

	canRead, _ := CanReadWorkspace(user, ws)
	if canRead {
		t.Fatalf("expected workspace access to be revoked")
	}
	isAdmin, _ := IsAdmin(user)
	if isAdmin {
		t.Fatalf("expected admin to be revoked")
	}
}
