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
