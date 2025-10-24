package models

import (
	"time"

	"gorm.io/gorm"
)

// Role represents a user role (admin, owner, editor, viewer)
type Role struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null" json:"name"`
	Description string         `json:"description"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
