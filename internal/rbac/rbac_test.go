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

func TestGroupGrantsProjectAccessTransitively(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	t.Cleanup(func() { enforcer = nil })

	user := uuid.New()
	group := uuid.New()
	project := uuid.New()

	if err := AddUserToGroup(user, group); err != nil {
		t.Fatalf("add user to group: %v", err)
	}
	if err := GrantGroupProjectAccess(group, project, "viewer"); err != nil {
		t.Fatalf("grant group: %v", err)
	}

	canRead, err := CanReadProject(user, project)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if !canRead {
		t.Fatalf("expected user to read project via group, got false")
	}
}

func TestGroupAdminTransitive(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	t.Cleanup(func() { enforcer = nil })

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
	t.Cleanup(func() { enforcer = nil })

	user := uuid.New()
	project := uuid.New()

	if err := GrantProjectAccess(user, project, "editor"); err != nil {
		t.Fatalf("grant project: %v", err)
	}

	canWrite, err := CanWriteProject(user, project)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if !canWrite {
		t.Fatalf("expected direct write policy to still match, got false")
	}
	projects, err := GetUserProjects(user)
	if err != nil {
		t.Fatalf("list project grants: %v", err)
	}
	if len(projects) != 1 || projects[0] != project {
		t.Fatalf("expected granted project in list, got %v", projects)
	}
	if err := RevokeProjectAccess(user, project); err != nil {
		t.Fatalf("revoke project: %v", err)
	}
	projects, err = GetUserProjects(user)
	if err != nil || len(projects) != 0 {
		t.Fatalf("expected no projects after revocation, got %v, %v", projects, err)
	}
}

func TestRemoveAllGroupPoliciesCleansEverything(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	t.Cleanup(func() { enforcer = nil })

	user := uuid.New()
	group := uuid.New()
	project := uuid.New()
	reg := uuid.New()

	_ = AddUserToGroup(user, group)
	_ = GrantGroupProjectAccess(group, project, "editor")
	_ = GrantGroupRegistryAccess(group, reg, "write")
	_ = MakeGroupAdmin(group)

	if err := RemoveAllGroupPolicies(group); err != nil {
		t.Fatalf("remove all: %v", err)
	}

	canRead, _ := CanReadProject(user, project)
	if canRead {
		t.Fatalf("expected project access to be revoked")
	}
	isAdmin, _ := IsAdmin(user)
	if isAdmin {
		t.Fatalf("expected admin to be revoked")
	}
	canWriteReg, _ := CanWriteRegistry(user, reg)
	if canWriteReg {
		t.Fatalf("expected registry write access to be revoked")
	}
}
