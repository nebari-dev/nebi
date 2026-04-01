package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LocalRegistry represents an OCI registry configured in the local CLI store.
// Credentials (passwords) are stored in the OS keychain, not in SQLite.
type LocalRegistry struct {
	ID        uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Name      string         `gorm:"uniqueIndex;not null" json:"name"`
	URL       string         `gorm:"not null" json:"url"`
	Username  string         `json:"username"`
	IsDefault bool           `gorm:"default:false" json:"is_default"`
	Namespace string         `json:"namespace"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName ensures GORM uses the "local_registries" table.
func (LocalRegistry) TableName() string {
	return "local_registries"
}

// BeforeCreate hook to generate UUID.
func (r *LocalRegistry) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
