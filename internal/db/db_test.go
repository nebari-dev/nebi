package db

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return database
}

func TestMigrateDetectsLegacyFederatedUsersWithoutIssuerSubject(t *testing.T) {
	database := openMigrationTestDB(t)
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

	err := Migrate(database)
	if err == nil {
		t.Fatal("expected legacy federated user migration error")
	}
	if !strings.Contains(err.Error(), "legacy federated users without issuer/subject bindings") {
		t.Fatalf("expected legacy federated user error, got %v", err)
	}
}

func TestMigrateAllowsFederatedUsersWithIssuerSubjectBinding(t *testing.T) {
	database := openMigrationTestDB(t)
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

	if err := Migrate(database); err != nil {
		t.Fatalf("expected migration to succeed: %v", err)
	}
}
