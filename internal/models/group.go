package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Group is an identity-provider group, synced from the "groups" claim of
// access tokens. Projects and registries can be granted to groups.
type Group struct {
	ID        uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Name      string         `gorm:"uniqueIndex;not null" json:"name"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (g *Group) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}
