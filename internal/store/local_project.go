package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LocalProject represents a project tracked by the CLI in its local SQLite database.
// This is separate from models.Project which is used by the server.
type LocalProject struct {
	ID             uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Name           string         `gorm:"not null" json:"name"`
	Status         string         `gorm:"not null;default:'ready'" json:"status"`
	Path           string         `gorm:"" json:"path,omitempty"`
	Source         string         `gorm:"default:'managed'" json:"source"`
	OriginID       string         `json:"origin_id,omitempty"`
	OriginName     string         `json:"origin_name,omitempty"`
	OriginTag      string         `json:"origin_tag,omitempty"`
	OriginAction   string         `json:"origin_action,omitempty"`
	OriginTomlHash string         `json:"origin_toml_hash,omitempty"`
	OriginLockHash string         `json:"origin_lock_hash,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName ensures GORM uses the "projects" table.
func (LocalProject) TableName() string {
	return "projects"
}

// BeforeCreate hook to generate UUID.
func (w *LocalProject) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}
