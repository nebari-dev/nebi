package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LocalPublication records a workspace publication to an OCI registry from the CLI.
type LocalPublication struct {
	ID          uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	WorkspaceID uuid.UUID      `gorm:"type:text;index;not null" json:"workspace_id"`
	RegistryID  uuid.UUID      `gorm:"type:text;index;not null" json:"registry_id"`
	Repository  string         `gorm:"not null" json:"repository"`
	Tag         string         `gorm:"not null" json:"tag"`
	Digest      string         `json:"digest"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName ensures GORM uses the "local_publications" table.
func (LocalPublication) TableName() string {
	return "local_publications"
}

// BeforeCreate hook to generate UUID.
func (p *LocalPublication) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
