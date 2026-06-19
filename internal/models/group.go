package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupSource identifies how a group entered the system.
type GroupSource string

const (
	GroupSourceNative GroupSource = "native"
	GroupSourceOIDC   GroupSource = "oidc"
)

// Group represents a named collection of users for permission grants.
type Group struct {
	ID          uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null" json:"name"`
	Description string         `json:"description"`
	Source      GroupSource    `gorm:"type:text;not null;default:native;index" json:"source"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (g *Group) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}
