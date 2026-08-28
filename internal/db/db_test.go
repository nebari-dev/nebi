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
		Username:     "legacy-oidc",
		Email:        "legacy@example.com",
		PasswordHash: "",
	}
	if err := database.Create(&legacyUser).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}

	if err := Migrate(database, false); err != nil {
		t.Fatalf("expected migration to leave legacy users for review-flow migration: %v", err)
	}
}

func TestMigrateAllowsFederatedUsersWithIssuerSubjectBinding(t *testing.T) {
	database := testDB(t)
	if err := database.AutoMigrate(&models.User{}, &models.FederatedIdentity{}); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	user := models.User{
		Username:     "bound-oidc",
		Email:        "bound@example.com",
		PasswordHash: "",
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
		"CREATE TABLE `workspaces` (`id` text PRIMARY KEY, `name` text NOT NULL, `package_manager` text NOT NULL)",
	).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	if err := Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if database.Migrator().HasColumn(&models.Workspace{}, "package_manager") {
		t.Fatal("expected package_manager column to be dropped")
	}

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	ws := models.Workspace{Name: "post-migration", OwnerID: owner.ID}
	if err := database.Create(&ws).Error; err != nil {
		t.Fatalf("create workspace after migration: %v", err)
	}
}

func TestMigrateDropsLegacyPackageManagerColumnWithReferencingRows(t *testing.T) {
	database := testDB(t)

	// Simulate a real pre-removal database: the SQLite driver emulates
	// DropColumn by rebuilding the table, and DROP TABLE on the old
	// workspaces violates the foreign keys held by referencing jobs rows
	// when enforcement is on (it is, via the DSN pragma).
	if err := database.Exec(
		"CREATE TABLE `users` (`id` text PRIMARY KEY, `username` text, `email` text, `password_hash` text NOT NULL)",
	).Error; err != nil {
		t.Fatalf("create legacy users table: %v", err)
	}
	if err := database.Exec(
		"CREATE TABLE `workspaces` (`id` text PRIMARY KEY, `name` text NOT NULL, `package_manager` text NOT NULL, `owner_id` text, CONSTRAINT `fk_workspaces_owner` FOREIGN KEY (`owner_id`) REFERENCES `users`(`id`))",
	).Error; err != nil {
		t.Fatalf("create legacy workspaces table: %v", err)
	}
	if err := database.Exec(
		"CREATE TABLE `jobs` (`id` text PRIMARY KEY, `workspace_id` text, `type` text NOT NULL, `status` text NOT NULL DEFAULT \"pending\", CONSTRAINT `fk_jobs_workspace` FOREIGN KEY (`workspace_id`) REFERENCES `workspaces`(`id`))",
	).Error; err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	if err := database.Exec(
		"INSERT INTO `users` (`id`, `username`, `email`, `password_hash`) VALUES ('user-1', 'owner', 'owner@example.com', 'x')",
	).Error; err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}
	if err := database.Exec(
		"INSERT INTO `workspaces` (`id`, `name`, `package_manager`, `owner_id`) VALUES ('ws-1', 'legacy', 'pixi', 'user-1')",
	).Error; err != nil {
		t.Fatalf("insert legacy workspace: %v", err)
	}
	if err := database.Exec(
		"INSERT INTO `jobs` (`id`, `workspace_id`, `type`) VALUES ('job-1', 'ws-1', 'create')",
	).Error; err != nil {
		t.Fatalf("insert referencing job: %v", err)
	}
	if err := Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if database.Migrator().HasColumn(&models.Workspace{}, "package_manager") {
		t.Fatal("expected package_manager column to be dropped")
	}

	var wsCount, jobCount int64
	if err := database.Table("workspaces").Count(&wsCount).Error; err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	if wsCount != 1 {
		t.Fatalf("expected 1 workspace to survive the migration, got %d", wsCount)
	}
	if err := database.Table("jobs").Count(&jobCount).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("expected 1 job to survive the migration, got %d", jobCount)
	}
}
