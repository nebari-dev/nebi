package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Permission represents user access to a project
type Permission struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	UserID    uuid.UUID      `gorm:"type:text;not null;index" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ProjectID uuid.UUID      `gorm:"type:text;not null;index" json:"project_id"`
	Project   Project        `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	RoleID    uint           `gorm:"not null" json:"role_id"`
	Role      Role           `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
