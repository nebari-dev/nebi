package db

import (
	"testing"

	"github.com/aktech/darb/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	err = db.AutoMigrate(&models.ServerConfig{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestGetOrCreateServerID_CreatesNewID(t *testing.T) {
	db := setupTestDB(t)

	// First call should create a new server ID
	serverID, err := GetOrCreateServerID(db)
	if err != nil {
		t.Fatalf("GetOrCreateServerID failed: %v", err)
	}

	// Verify it's a valid UUID
	_, err = uuid.Parse(serverID)
	if err != nil {
		t.Errorf("server ID is not a valid UUID: %v", err)
	}

	// Verify it was stored in the database
	var config models.ServerConfig
	err = db.Where("key = ?", models.ServerConfigKeyServerID).First(&config).Error
	if err != nil {
		t.Fatalf("failed to query server config: %v", err)
	}

	if config.Value != serverID {
		t.Errorf("stored server ID mismatch: got %s, want %s", config.Value, serverID)
	}
}

func TestGetOrCreateServerID_ReturnsExistingID(t *testing.T) {
	db := setupTestDB(t)

	// Create an existing server ID
	existingID := "existing-server-id-123"
	config := models.ServerConfig{
		Key:   models.ServerConfigKeyServerID,
		Value: existingID,
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("failed to create existing server ID: %v", err)
	}

	// GetOrCreateServerID should return the existing ID
	serverID, err := GetOrCreateServerID(db)
	if err != nil {
		t.Fatalf("GetOrCreateServerID failed: %v", err)
	}

	if serverID != existingID {
		t.Errorf("expected existing ID %s, got %s", existingID, serverID)
	}
}

func TestGetOrCreateServerID_Idempotent(t *testing.T) {
	db := setupTestDB(t)

	// Call multiple times
	id1, err := GetOrCreateServerID(db)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	id2, err := GetOrCreateServerID(db)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	id3, err := GetOrCreateServerID(db)
	if err != nil {
		t.Fatalf("third call failed: %v", err)
	}

	// All calls should return the same ID
	if id1 != id2 || id2 != id3 {
		t.Errorf("server ID changed between calls: %s, %s, %s", id1, id2, id3)
	}
}

func TestGetServerID_ReturnsExistingID(t *testing.T) {
	db := setupTestDB(t)

	// First create a server ID
	createdID, err := GetOrCreateServerID(db)
	if err != nil {
		t.Fatalf("GetOrCreateServerID failed: %v", err)
	}

	// GetServerID should return the same ID
	retrievedID, err := GetServerID(db)
	if err != nil {
		t.Fatalf("GetServerID failed: %v", err)
	}

	if retrievedID != createdID {
		t.Errorf("GetServerID returned different ID: got %s, want %s", retrievedID, createdID)
	}
}

func TestGetServerID_ErrorsWhenNotInitialized(t *testing.T) {
	db := setupTestDB(t)

	// GetServerID should error when no server ID exists
	_, err := GetServerID(db)
	if err == nil {
		t.Error("GetServerID should error when server ID is not initialized")
	}
}
