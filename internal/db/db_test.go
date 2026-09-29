package db

import (
	"path/filepath"
	"testing"

	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(t.TempDir(), "test.db"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return database
}

func TestMigrateAllowsLegacyFederatedUsersWithoutIssuerSubject(t *testing.T) {
	database := testDB(t)
	if err := database.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	legacyUser := models.User{
		Username: "legacy-oidc",
		Email:    "legacy@example.com",
	}
	if err := database.Create(&legacyUser).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}

	if err := Migrate(database, false); err != nil {
		t.Fatalf("expected migration to leave legacy users in place: %v", err)
	}
}

func TestMigrateAllowsFederatedUsersWithIssuerSubjectBinding(t *testing.T) {
	database := testDB(t)
	if err := database.AutoMigrate(&models.User{}, &models.FederatedIdentity{}); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	user := models.User{
		Username: "bound-oidc",
		Email:    "bound@example.com",
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := database.Create(&models.FederatedIdentity{
		UserID:  user.ID,
		Issuer:  "https://issuer.example.com",
		Subject: "subject",
	}).Error; err != nil {
		t.Fatalf("create federated identity: %v", err)
	}

	if err := Migrate(database, false); err != nil {
		t.Fatalf("expected migration to succeed: %v", err)
	}
}

func countDefaultRegistry(t *testing.T, database *gorm.DB) int64 {
	t.Helper()
	var count int64
	database.Model(&models.OCIRegistry{}).Where("name = ?", "nebari-environments").Count(&count)
	return count
}

func TestMigrate_SeedsDefaultRegistry(t *testing.T) {
	database := testDB(t)

	if err := Migrate(database, true); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := countDefaultRegistry(t, database); got != 1 {
		t.Errorf("expected default registry seeded, count=%d", got)
	}

	// Marker must exist so the seed is one-time.
	var marker models.SystemSetting
	if err := database.Where("key = ?", "default_registry_seeded").First(&marker).Error; err != nil {
		t.Errorf("expected seed marker, got error: %v", err)
	}
}

func TestMigrate_SeedDisabled(t *testing.T) {
	database := testDB(t)

	if err := Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := countDefaultRegistry(t, database); got != 0 {
		t.Errorf("expected no default registry with seeding disabled, count=%d", got)
	}
}

func TestMigrate_DoesNotReseedAfterDelete(t *testing.T) {
	database := testDB(t)

	if err := Migrate(database, true); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Admin deliberately deletes the default registry.
	database.Where("name = ?", "nebari-environments").Delete(&models.OCIRegistry{})

	if err := Migrate(database, true); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := countDefaultRegistry(t, database); got != 0 {
		t.Errorf("deleted default registry was re-seeded, count=%d", got)
	}
}

func TestMigrate_BackfillsMarkerForExistingRow(t *testing.T) {
	database := testDB(t)

	// Simulate a pre-feature database: registry row exists, no marker table content.
	if err := database.AutoMigrate(&models.OCIRegistry{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	database.Create(&models.OCIRegistry{Name: "nebari-environments", URL: "quay.io", Namespace: "nebari_environments", IsDefault: true})

	if err := Migrate(database, true); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := countDefaultRegistry(t, database); got != 1 {
		t.Errorf("expected exactly 1 default registry after backfill, count=%d", got)
	}
	var marker models.SystemSetting
	if err := database.Where("key = ?", "default_registry_seeded").First(&marker).Error; err != nil {
		t.Errorf("expected marker backfilled, got error: %v", err)
	}
}

func TestMigrateDropsLegacyPackageManagerColumn(t *testing.T) {
	database := testDB(t)

	// Simulate a database created before the package_manager column was
	// removed: NOT NULL with no default, which would break inserts if left.
	if err := database.Exec(
		"CREATE TABLE `projects` (`id` text PRIMARY KEY, `name` text NOT NULL, `package_manager` text NOT NULL)",
	).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	if err := Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if database.Migrator().HasColumn(&models.Project{}, "package_manager") {
		t.Fatal("expected package_manager column to be dropped")
	}

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	project := models.Project{Name: "post-migration", OwnerID: owner.ID}
	if err := database.Create(&project).Error; err != nil {
		t.Fatalf("create project after migration: %v", err)
	}
}

func TestMigrateDropsLegacyPackageManagerColumnWithReferencingRows(t *testing.T) {
	database := testDB(t)

	// Simulate a real pre-removal database: the SQLite driver emulates
	// DropColumn by rebuilding the table, and DROP TABLE on the old
	// projects violates the foreign keys held by referencing jobs rows
	// when enforcement is on (it is, via the DSN pragma).
	if err := database.Exec(
		"CREATE TABLE `users` (`id` text PRIMARY KEY, `username` text, `email` text, `password_hash` text NOT NULL)",
	).Error; err != nil {
		t.Fatalf("create legacy users table: %v", err)
	}
	if err := database.Exec(
		"CREATE TABLE `projects` (`id` text PRIMARY KEY, `name` text NOT NULL, `package_manager` text NOT NULL, `owner_id` text, CONSTRAINT `fk_projects_owner` FOREIGN KEY (`owner_id`) REFERENCES `users`(`id`))",
	).Error; err != nil {
		t.Fatalf("create legacy projects table: %v", err)
	}
	if err := database.Exec(
		"CREATE TABLE `jobs` (`id` text PRIMARY KEY, `project_id` text, `type` text NOT NULL, `status` text NOT NULL DEFAULT \"pending\", CONSTRAINT `fk_jobs_project` FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`))",
	).Error; err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	if err := database.Exec(
		"INSERT INTO `users` (`id`, `username`, `email`, `password_hash`) VALUES ('user-1', 'owner', 'owner@example.com', 'x')",
	).Error; err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}
	if err := database.Exec(
		"INSERT INTO `projects` (`id`, `name`, `package_manager`, `owner_id`) VALUES ('ws-1', 'legacy', 'pixi', 'user-1')",
	).Error; err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	if err := database.Exec(
		"INSERT INTO `jobs` (`id`, `project_id`, `type`) VALUES ('job-1', 'ws-1', 'create')",
	).Error; err != nil {
		t.Fatalf("insert referencing job: %v", err)
	}
	if err := Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if database.Migrator().HasColumn(&models.Project{}, "package_manager") {
		t.Fatal("expected package_manager column to be dropped")
	}

	var projectCount, jobCount int64
	if err := database.Table("projects").Count(&projectCount).Error; err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if projectCount != 1 {
		t.Fatalf("expected 1 project to survive the migration, got %d", projectCount)
	}
	if err := database.Table("jobs").Count(&jobCount).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("expected 1 job to survive the migration, got %d", jobCount)
	}
}

func TestMigrateRemovesBuiltInUserManagementState(t *testing.T) {
	database := testDB(t)

	// A database from before auth was delegated to the identity provider:
	// password users, native and OIDC groups with grants, the review queue
	// and the reconciliation status table.
	for _, stmt := range []string{
		"CREATE TABLE `users` (`id` text PRIMARY KEY, `username` text NOT NULL, `password_hash` text NOT NULL, `email` text NOT NULL, `avatar_url` text, `created_at` datetime, `updated_at` datetime, `deleted_at` datetime)",
		"CREATE UNIQUE INDEX `idx_users_username` ON `users`(`username`)",
		"CREATE UNIQUE INDEX `idx_users_email` ON `users`(`email`)",
		"CREATE TABLE `groups` (`id` text PRIMARY KEY, `name` text NOT NULL, `description` text, `source` text NOT NULL DEFAULT 'native', `created_at` datetime, `updated_at` datetime, `deleted_at` datetime)",
		"CREATE UNIQUE INDEX `idx_groups_name` ON `groups`(`name`)",
		"CREATE TABLE `group_members` (`group_id` text, `user_id` text, `created_at` datetime, PRIMARY KEY (`group_id`,`user_id`), CONSTRAINT `fk_group_members_user` FOREIGN KEY (`user_id`) REFERENCES `users`(`id`), CONSTRAINT `fk_group_members_group` FOREIGN KEY (`group_id`) REFERENCES `groups`(`id`))",
		"CREATE TABLE `federated_identity_reviews` (`id` text PRIMARY KEY)",
		"CREATE TABLE `auth_reconciliation_statuses` (`id` integer PRIMARY KEY)",
		"CREATE TABLE `casbin_rule` (`id` integer PRIMARY KEY AUTOINCREMENT, `ptype` text, `v0` text, `v1` text, `v2` text, `v3` text, `v4` text, `v5` text)",
		"INSERT INTO `users` (`id`, `username`, `password_hash`, `email`) VALUES ('11111111-1111-1111-1111-111111111111', 'alice', 'hash', 'alice@example.com')",
		"INSERT INTO `groups` (`id`, `name`, `source`) VALUES ('22222222-2222-2222-2222-222222222222', 'ops', 'native'), ('33333333-3333-3333-3333-333333333333', 'data-science', 'oidc')",
		"INSERT INTO `group_members` (`group_id`, `user_id`) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111'), ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111')",
		"INSERT INTO `casbin_rule` (`ptype`, `v0`, `v1`, `v2`) VALUES ('g', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', ''), ('p', '22222222-2222-2222-2222-222222222222', 'admin', 'admin'), ('g', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', ''), ('p', '33333333-3333-3333-3333-333333333333', 'project:x', 'read')",
	} {
		if err := database.Exec(stmt).Error; err != nil {
			t.Fatalf("seed legacy schema (%s): %v", stmt, err)
		}
	}

	if err := Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Idempotent.
	if err := Migrate(database, false); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	for _, col := range []struct {
		model  any
		column string
	}{
		{&models.User{}, "password_hash"},
		{&models.Group{}, "source"},
		{&models.Group{}, "description"},
	} {
		if database.Migrator().HasColumn(col.model, col.column) {
			t.Errorf("expected column %s to be dropped", col.column)
		}
	}
	for _, table := range []string{"federated_identity_reviews", "auth_reconciliation_statuses"} {
		if database.Migrator().HasTable(table) {
			t.Errorf("expected table %s to be dropped", table)
		}
	}

	var groups []models.Group
	database.Find(&groups)
	if len(groups) != 1 || groups[0].Name != "data-science" {
		t.Fatalf("expected only the IdP group to survive, got %+v", groups)
	}
	var members int64
	database.Model(&models.GroupMember{}).Count(&members)
	if members != 1 {
		t.Fatalf("expected 1 membership to survive, got %d", members)
	}
	const nativeGroupID = "22222222-2222-2222-2222-222222222222"
	var rules []struct{ Ptype, V0, V1 string }
	database.Table("casbin_rule").Find(&rules)
	if len(rules) != 2 {
		t.Fatalf("expected the 2 IdP group policies to survive, got %+v", rules)
	}
	for _, r := range rules {
		if r.V0 == nativeGroupID || r.V1 == nativeGroupID {
			t.Fatalf("native group policy survived: %+v", r)
		}
	}

	// Users keep their unique constraints and accept inserts without a
	// password hash.
	if err := database.Create(&models.User{Username: "bob", Email: "bob@example.com"}).Error; err != nil {
		t.Fatalf("create user after migration: %v", err)
	}
	if err := database.Create(&models.User{Username: "alice", Email: "other@example.com"}).Error; err == nil {
		t.Fatal("expected unique username constraint to survive the migration")
	}
}
