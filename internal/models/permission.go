package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Permission represents user access to a workspace
type Permission struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	UserID      uuid.UUID      `gorm:"type:text;not null;index" json:"user_id"`
	User        User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	WorkspaceID uuid.UUID      `gorm:"type:text;not null;index" json:"workspace_id"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	RoleID        uint           `gorm:"not null" json:"role_id"`
	Role          Role           `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
