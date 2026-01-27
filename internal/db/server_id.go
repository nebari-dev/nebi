package db

import (
	"fmt"
	"log/slog"

	"github.com/aktech/darb/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetOrCreateServerID retrieves the server ID from the database,
// or generates and stores a new one if it doesn't exist.
// This should be called during server startup after migrations.
func GetOrCreateServerID(db *gorm.DB) (string, error) {
	var config models.ServerConfig

	// Try to find existing server ID
	err := db.Where("key = ?", models.ServerConfigKeyServerID).First(&config).Error
	if err == nil {
		slog.Info("Found existing server ID", "server_id", config.Value)
		return config.Value, nil
	}

	if err != gorm.ErrRecordNotFound {
		return "", fmt.Errorf("failed to query server config: %w", err)
	}

	// Generate new server ID
	serverID := uuid.New().String()
	config = models.ServerConfig{
		Key:   models.ServerConfigKeyServerID,
		Value: serverID,
	}

	if err := db.Create(&config).Error; err != nil {
		return "", fmt.Errorf("failed to create server ID: %w", err)
	}

	slog.Info("Generated new server ID", "server_id", serverID)
	return serverID, nil
}

// GetServerID retrieves the server ID from the database.
// Returns an error if the server ID has not been initialized.
func GetServerID(db *gorm.DB) (string, error) {
	var config models.ServerConfig

	err := db.Where("key = ?", models.ServerConfigKeyServerID).First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("server ID not initialized")
		}
		return "", fmt.Errorf("failed to query server config: %w", err)
	}

	return config.Value, nil
}
