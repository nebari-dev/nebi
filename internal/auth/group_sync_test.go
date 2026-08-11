package auth

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
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
		&models.AuthReconciliationStatus{},
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
	for _, g := range groups {
		if g.Source != models.GroupSourceOIDC {
			t.Errorf("group %q expected source oidc, got %q", g.Name, g.Source)
		}
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

func TestOIDCGroupSync_DoesNotTouchNativeMemberships(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	native := models.Group{Name: "native-grp", Source: models.GroupSourceNative}
	db.Create(&native)
	db.Create(&models.GroupMember{GroupID: native.ID, UserID: u.ID})
	_ = rbac.AddUserToGroup(u.ID, native.ID)

	_ = syncOIDCGroups(db, u.ID, []string{"x"}, rbac.NewDefaultProvider())

	var mem models.GroupMember
	if err := db.Where("group_id = ? AND user_id = ?", native.ID, u.ID).First(&mem).Error; err != nil {
		t.Fatalf("native membership should be untouched, err=%v", err)
	}
	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 2 {
		t.Fatalf("expected 2 memberships (1 native + 1 oidc), got %d: %v", len(memberships), memberships)
	}
	nativeStillPresent := false
	for _, id := range memberships {
		if id == native.ID {
			nativeStillPresent = true
			break
		}
	}
	if !nativeStillPresent {
		t.Fatalf("native group %s missing from casbin memberships: %v", native.ID, memberships)
	}
}

func TestOIDCGroupSync_RefusesToMergeIntoNativeGroup(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	// Operator pre-creates a native group with a name that an IdP could collide with.
	native := models.Group{Name: "engineering", Source: models.GroupSourceNative}
	if err := db.Create(&native).Error; err != nil {
		t.Fatalf("seed native: %v", err)
	}

	// OIDC claim arrives with the same name.
	if err := syncOIDCGroups(db, u.ID, []string{"engineering"}, rbac.NewDefaultProvider()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Alice must NOT be a member of the native group via DB.
	var mem models.GroupMember
	err := db.Where("group_id = ? AND user_id = ?", native.ID, u.ID).First(&mem).Error
	if err == nil {
		t.Fatalf("expected no GroupMember row for native group, found one")
	}

	// And NOT a member via Casbin.
	memberships, _ := rbac.GetUserGroups(u.ID)
	for _, id := range memberships {
		if id == native.ID {
			t.Fatalf("expected user NOT to be in casbin grouping rule for native group, got %v", memberships)
		}
	}

	// The native group's source must remain unchanged.
	var refetched models.Group
	if err := db.First(&refetched, "id = ?", native.ID).Error; err != nil {
		t.Fatalf("refetch native: %v", err)
	}
	if refetched.Source != models.GroupSourceNative {
		t.Fatalf("native group's source was reclassified to %q", refetched.Source)
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

func TestOIDCGroupSync_RecordsSuccessAndFailureStatus(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	provider := &stubRBACProvider{}
	if err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var status models.AuthReconciliationStatus
	if err := db.First(&status, "user_id = ? AND kind = ?", u.ID, string(authReconciliationOIDCGroups)).Error; err != nil {
		t.Fatalf("load success status: %v", err)
	}
	if status.LastSuccessAt == nil {
		t.Fatal("expected last success timestamp")
	}
	if status.ConsecutiveFailures != 0 {
		t.Fatalf("expected failures reset after success, got %d", status.ConsecutiveFailures)
	}
	if status.DesiredGroupsJSON != `["engineering"]` {
		t.Fatalf("expected desired groups to be stored, got %q", status.DesiredGroupsJSON)
	}

	wantErr := errors.New("casbin list failed")
	provider.getUserGroupsErr = wantErr
	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected rbac list error, got %v", err)
	}

	if err := db.First(&status, "user_id = ? AND kind = ?", u.ID, string(authReconciliationOIDCGroups)).Error; err != nil {
		t.Fatalf("load failure status: %v", err)
	}
	if status.LastFailureAt == nil {
		t.Fatal("expected last failure timestamp")
	}
	if status.ConsecutiveFailures != 1 {
		t.Fatalf("expected one consecutive failure, got %d", status.ConsecutiveFailures)
	}
	if status.LastFailureSource != string(authReconciliationFailureSourceLocal) {
		t.Fatalf("expected local failure source, got %q", status.LastFailureSource)
	}
	if !strings.Contains(status.LastError, wantErr.Error()) {
		t.Fatalf("expected last error to contain %q, got %q", wantErr, status.LastError)
	}
}

func TestOIDCGroupSync_ReturnsStatusCreateFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	wantErr := errors.New("status create failed")
	name := registerDBTableFailureCallback(t, db, "create", "auth_reconciliation_statuses", wantErr)
	defer db.Callback().Create().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, &stubRBACProvider{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected status create error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsStatusUpdateFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	provider := &stubRBACProvider{}
	if err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider); err != nil {
		t.Fatalf("seed status: %v", err)
	}
	oldSuccess := time.Now().UTC().Add(-authReconciliationSuccessRefreshAfter() - time.Second)
	if err := db.Model(&models.AuthReconciliationStatus{}).
		Where("user_id = ? AND kind = ?", u.ID, string(authReconciliationOIDCGroups)).
		Update("last_success_at", oldSuccess).Error; err != nil {
		t.Fatalf("age status: %v", err)
	}

	wantErr := errors.New("status update failed")
	name := registerDBTableFailureCallback(t, db, "update", "auth_reconciliation_statuses", wantErr)
	defer db.Callback().Update().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected status update error, got %v", err)
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

func TestOIDCGroupSync_ReturnsMembershipLookupFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	group := models.Group{Name: "engineering", Source: models.GroupSourceOIDC}
	db.Create(&group)

	wantErr := errors.New("membership lookup failed")
	name := registerDBTableFailureCallbackAfter(t, db, "query", "group_members", 1, wantErr)
	defer db.Callback().Query().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, &stubRBACProvider{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected membership lookup error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsMembershipCreateFailure(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	group := models.Group{Name: "engineering", Source: models.GroupSourceOIDC}
	db.Create(&group)

	wantErr := errors.New("membership create failed")
	name := registerDBTableFailureCallback(t, db, "create", "group_members", wantErr)
	defer db.Callback().Create().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, &stubRBACProvider{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected membership create error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsOriginalErrorWhenFailureStatusCreateFails(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	wantErr := errors.New("casbin list failed")
	provider := &stubRBACProvider{getUserGroupsErr: wantErr}
	name := registerDBTableFailureCallback(t, db, "create", "auth_reconciliation_statuses", errors.New("status create failed"))
	defer db.Callback().Create().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected original reconciliation error, got %v", err)
	}
}

func TestOIDCGroupSync_ReturnsOriginalErrorWhenFailureStatusUpdateFails(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	provider := &stubRBACProvider{}
	if err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	wantErr := errors.New("casbin list failed")
	provider.getUserGroupsErr = wantErr
	name := registerDBTableFailureCallback(t, db, "update", "auth_reconciliation_statuses", errors.New("status update failed"))
	defer db.Callback().Update().Remove(name)

	err := syncOIDCGroups(db, u.ID, []string{"engineering"}, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected original reconciliation error, got %v", err)
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
