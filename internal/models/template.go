package models

import (
	"time"

	"gorm.io/gorm"
)

// Template represents a pre-configured workspace template
type Template struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null" json:"name"`
	Description string         `json:"description"`
	ConfigJSON  string         `gorm:"type:text;not null" json:"config_json"` // JSON config for workspace
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
