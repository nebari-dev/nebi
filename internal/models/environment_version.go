package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EnvironmentVersion represents a snapshot of environment files at a point in time
type EnvironmentVersion struct {
	ID              uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	EnvironmentID   uuid.UUID      `gorm:"type:text;not null;index:idx_env_version" json:"environment_id"`
	Environment     Environment    `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`

	// Version tracking
	VersionNumber   int            `gorm:"not null;index:idx_env_version" json:"version_number"` // Auto-incrementing per environment

	// File contents (stored as TEXT in database)
	LockFileContent string         `gorm:"type:text;not null" json:"lock_file_content"`      // pixi.lock content
	ManifestContent string         `gorm:"type:text;not null" json:"manifest_content"`       // pixi.toml content
	PackageMetadata string         `gorm:"type:text;not null" json:"package_metadata"`       // JSON of package list

	// Context
	JobID           *uuid.UUID     `gorm:"type:text;index" json:"job_id,omitempty"`          // Job that triggered this version
	Job             *Job           `gorm:"foreignKey:JobID" json:"job,omitempty"`
	CreatedBy       uuid.UUID      `gorm:"type:text;not null" json:"created_by"`             // User who triggered the change
	CreatedByUser   User           `gorm:"foreignKey:CreatedBy" json:"created_by_user,omitempty"`
	Description     string         `gorm:"type:text" json:"description,omitempty"`           // Optional description of changes

	// Timestamps
	CreatedAt       time.Time      `json:"created_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID and version number
func (ev *EnvironmentVersion) BeforeCreate(tx *gorm.DB) error {
	if ev.ID == uuid.Nil {
		ev.ID = uuid.New()
	}

	// Auto-increment version number for this environment
	if ev.VersionNumber == 0 {
		var maxVersion struct {
			MaxVersion *int
		}
		tx.Model(&EnvironmentVersion{}).
			Select("MAX(version_number) as max_version").
			Where("environment_id = ?", ev.EnvironmentID).
			Scan(&maxVersion)

		if maxVersion.MaxVersion == nil {
			ev.VersionNumber = 1
		} else {
			ev.VersionNumber = *maxVersion.MaxVersion + 1
		}
	}

	return nil
}
