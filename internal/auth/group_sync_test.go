package auth

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func syncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Group{},
		&models.GroupMember{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("rbac: %v", err)
	}
	return db
}

func TestOIDCGroupSync_CreatesGroupAndMembership(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	if err := syncOIDCGroups(db, u.ID, []string{"data-science", "admins"}, rbac.NewDefaultProvider()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var groups []models.Group
	db.Find(&groups)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 2 {
		t.Fatalf("expected 2 casbin memberships, got %d", len(memberships))
	}
}

func TestOIDCGroupSync_StripsLeadingSlashAndDedups(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	// Keycloak can emit the same group twice in the `groups` claim: once as a
	// full path ("/developer", from a full.path=true mapper) and once as the
	// bare name ("developer"). Both refer to one group and must collapse.
	if err := syncOIDCGroups(db, u.ID, []string{"/developer", "developer"}, rbac.NewDefaultProvider()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var groups []models.Group
	db.Find(&groups)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group after dedup, got %d: %+v", len(groups), groups)
	}
	if groups[0].Name != "developer" {
		t.Errorf("expected normalized name 'developer', got %q", groups[0].Name)
	}

	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 1 {
		t.Fatalf("expected 1 casbin membership, got %d", len(memberships))
	}
}

func TestOIDCGroupSync_RemovesStaleMemberships(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	_ = syncOIDCGroups(db, u.ID, []string{"x", "y"}, rbac.NewDefaultProvider())

	if err := syncOIDCGroups(db, u.ID, []string{"x"}, rbac.NewDefaultProvider()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 1 {
		t.Fatalf("expected 1 membership after reconcile, got %d", len(memberships))
	}
}

func TestOIDCGroupSync_KeepsZeroMemberGroups(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	_ = syncOIDCGroups(db, u.ID, []string{"keep-me"}, rbac.NewDefaultProvider())
	_ = syncOIDCGroups(db, u.ID, []string{}, rbac.NewDefaultProvider()) // user dropped from the group

	var g models.Group
	if err := db.First(&g, "name = ?", "keep-me").Error; err != nil {
		t.Fatalf("expected group 'keep-me' to still exist, err=%v", err)
	}
}

func TestOIDCGroupSync_ReplacesMembershipsOfPreexistingGroups(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	// A group created earlier (e.g. by another user's token) is joined, not
	// duplicated, and a membership missing from the claim is removed.
	existing := models.Group{Name: "engineering"}
	db.Create(&existing)
	stale := models.Group{Name: "old-team"}
	db.Create(&stale)
	db.Create(&models.GroupMember{GroupID: stale.ID, UserID: u.ID})
	_ = rbac.AddUserToGroup(u.ID, stale.ID)

	if err := syncOIDCGroups(db, u.ID, []string{"engineering"}, rbac.NewDefaultProvider()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var groups int64
	db.Model(&models.Group{}).Count(&groups)
	if groups != 2 {
		t.Fatalf("expected no new group, got %d groups", groups)
	}
	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 1 || memberships[0] != existing.ID {
		t.Fatalf("expected only the engineering membership, got %v", memberships)
	}
}

func TestOIDCGroupSync_ToleratesConcurrentGroupCreate(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	// Simulate another request creating the group between the lookup and
	// the insert.
	racer := uuid.New()
	injected := false
	const name = "race-group"
	if err := db.Callback().Create().Before("gorm:create").Register("test:race_group_create", func(d *gorm.DB) {
		if injected || d.Statement.Table != "groups" {
			return
		}
		injected = true
		now := time.Now()
		if err := d.Session(&gorm.Session{NewDB: true}).Exec(
			"INSERT INTO groups (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)", racer, name, now, now,
		).Error; err != nil {
			t.Fatalf("inject concurrent group: %v", err)
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:race_group_create") })

	if err := syncOIDCGroups(db, u.ID, []string{name}, rbac.NewDefaultProvider()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !injected {
		t.Fatal("expected the concurrent insert to be injected")
	}
	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 1 || memberships[0] != racer {
		t.Fatalf("expected membership in the concurrently created group %s, got %v", racer, memberships)
	}
}

func TestOIDCGroupSync_ReturnsRBACAddFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	wantErr := errors.New("casbin add failed")
	provider := &stubRBACProvider{addUserToGroupErr: wantErr}

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected rbac add error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsRBACListFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	wantErr := errors.New("casbin list failed")
	provider := &stubRBACProvider{getUserGroupsErr: wantErr}

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected rbac list error, got %v", err)
	}
}

func TestOIDCGroupSync_RetainsStaleMembershipWhenRBACRemoveFails(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	provider := &stubRBACProvider{}
	if err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	var group models.Group
	if err := db.First(&group, "name = ?", "engineering").Error; err != nil {
		t.Fatalf("load group: %v", err)
	}

	wantErr := errors.New("casbin remove failed")
	provider.removeUserFromGroupErr = wantErr
	err := syncOIDCGroups(db, u.ID, nil, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected rbac remove error, got %v", err)
	}

	var count int64
	db.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", group.ID, u.ID).
		Count(&count)
	if count != 1 {
		t.Fatalf("expected stale membership to remain for retry, got %d rows", count)
	}
}

func TestOIDCGroupSync_ReturnsGroupLookupFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	wantErr := errors.New("group lookup failed")
	name := registerDBTableFailureCallback(t, db, "query", "groups", wantErr)
	defer db.Callback().Query().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, &stubRBACProvider{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected group lookup error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsGroupCreateFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	wantErr := errors.New("group create failed")
	name := registerDBTableFailureCallback(t, db, "create", "groups", wantErr)
	defer db.Callback().Create().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, &stubRBACProvider{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected group create error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsMembershipCreateFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	group := models.Group{Name: "engineering"}
	db.Create(&group)

	wantErr := errors.New("membership create failed")
	name := registerDBTableFailureCallback(t, db, "create", "group_members", wantErr)
	defer db.Callback().Create().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, &stubRBACProvider{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected membership create error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsDBQueryFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	wantErr := errors.New("query failed")
	name := registerDBFailureCallback(t, db, "query", wantErr)
	defer db.Callback().Query().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, &stubRBACProvider{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected db query error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsDBDeleteFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	provider := &stubRBACProvider{}
	if err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	wantErr := errors.New("delete failed")
	name := registerDBFailureCallback(t, db, "delete", wantErr)
	defer db.Callback().Delete().Remove(name)

	err := syncOIDCGroups(db, u.ID, nil, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected db delete error, got %v", err)
	}
}

func registerDBFailureCallback(t *testing.T, db *gorm.DB, op string, err error) string {
	t.Helper()
	name := "test:fail:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	switch op {
	case "query":
		if registerErr := db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
			tx.AddError(err)
		}); registerErr != nil {
			t.Fatalf("register query callback: %v", registerErr)
		}
	case "delete":
		if registerErr := db.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			tx.AddError(err)
		}); registerErr != nil {
			t.Fatalf("register delete callback: %v", registerErr)
		}
	default:
		t.Fatalf("unsupported callback op %q", op)
	}
	return name
}

func registerDBTableFailureCallback(t *testing.T, db *gorm.DB, op string, table string, err error) string {
	t.Helper()
	return registerDBTableFailureCallbackAfter(t, db, op, table, 0, err)
}

func registerDBTableFailureCallbackAfter(t *testing.T, db *gorm.DB, op string, table string, skipMatches int, err error) string {
	t.Helper()
	name := "test:fail:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	matches := 0
	failForTable := func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == table {
			if matches < skipMatches {
				matches++
				return
			}
			tx.AddError(err)
		}
	}
	switch op {
	case "query":
		if registerErr := db.Callback().Query().Before("gorm:query").Register(name, failForTable); registerErr != nil {
			t.Fatalf("register query callback: %v", registerErr)
		}
	case "create":
		if registerErr := db.Callback().Create().Before("gorm:create").Register(name, failForTable); registerErr != nil {
			t.Fatalf("register create callback: %v", registerErr)
		}
	case "delete":
		if registerErr := db.Callback().Delete().Before("gorm:delete").Register(name, failForTable); registerErr != nil {
			t.Fatalf("register delete callback: %v", registerErr)
		}
	case "update":
		if registerErr := db.Callback().Update().Before("gorm:update").Register(name, failForTable); registerErr != nil {
			t.Fatalf("register update callback: %v", registerErr)
		}
	default:
		t.Fatalf("unsupported callback op %q", op)
	}
	return name
}
