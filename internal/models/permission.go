package models

import (
	"time"

	"gorm.io/gorm"
)

// Permission represents user access to an environment
type Permission struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	User          User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	EnvironmentID uint           `gorm:"not null;index" json:"environment_id"`
	Environment   Environment    `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	RoleID        uint           `gorm:"not null" json:"role_id"`
	Role          Role           `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
