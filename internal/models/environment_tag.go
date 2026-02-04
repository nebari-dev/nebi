package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EnvironmentTag represents a named pointer to a specific version of an environment.
// Tags are mutable — pushing the same tag again re-points it to the new version.
type EnvironmentTag struct {
	ID            uuid.UUID    `gorm:"type:text;primary_key" json:"id"`
	EnvironmentID uuid.UUID    `gorm:"type:text;not null;uniqueIndex:idx_env_tag" json:"environment_id"`
	Environment   *Environment `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	Tag           string       `gorm:"not null;uniqueIndex:idx_env_tag" json:"tag"`
	VersionNumber int          `gorm:"not null" json:"version_number"`
	CreatedBy     uuid.UUID    `gorm:"type:text;not null" json:"created_by"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// BeforeCreate hook to generate UUID
func (et *EnvironmentTag) BeforeCreate(tx *gorm.DB) error {
	if et.ID == uuid.Nil {
		et.ID = uuid.New()
	}
	return nil
}
