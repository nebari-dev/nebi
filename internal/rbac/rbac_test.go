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
