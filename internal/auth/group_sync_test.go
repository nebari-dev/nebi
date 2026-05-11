package auth

import (
	"log/slog"
	"testing"

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
	if err := db.AutoMigrate(&models.User{}, &models.Group{}, &models.GroupMember{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("rbac: %v", err)
	}
	return db
}

func TestSyncOIDCGroups_CreatesGroupAndMembership(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	if err := SyncOIDCGroups(db, u.ID, []string{"data-science", "admins"}); err != nil {
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

func TestSyncOIDCGroups_RemovesStaleMemberships(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	_ = SyncOIDCGroups(db, u.ID, []string{"x", "y"})

	if err := SyncOIDCGroups(db, u.ID, []string{"x"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 1 {
		t.Fatalf("expected 1 membership after reconcile, got %d", len(memberships))
	}
}

func TestSyncOIDCGroups_KeepsZeroMemberGroups(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	_ = SyncOIDCGroups(db, u.ID, []string{"keep-me"})
	_ = SyncOIDCGroups(db, u.ID, []string{}) // user dropped from the group

	var g models.Group
	if err := db.First(&g, "name = ?", "keep-me").Error; err != nil {
		t.Fatalf("expected group 'keep-me' to still exist, err=%v", err)
	}
}

func TestSyncOIDCGroups_DoesNotTouchNativeMemberships(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	native := models.Group{Name: "native-grp", Source: models.GroupSourceNative}
	db.Create(&native)
	db.Create(&models.GroupMember{GroupID: native.ID, UserID: u.ID})
	_ = rbac.AddUserToGroup(u.ID, native.ID)

	_ = SyncOIDCGroups(db, u.ID, []string{"x"})

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

func TestSyncOIDCGroups_RefusesToMergeIntoNativeGroup(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	// Operator pre-creates a native group with a name that an IdP could collide with.
	native := models.Group{Name: "engineering", Source: models.GroupSourceNative}
	if err := db.Create(&native).Error; err != nil {
		t.Fatalf("seed native: %v", err)
	}

	// OIDC claim arrives with the same name.
	if err := SyncOIDCGroups(db, u.ID, []string{"engineering"}); err != nil {
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
