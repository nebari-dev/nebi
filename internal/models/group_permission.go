package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupPermission is the sibling of `Permission` (which is per-user) and represents
// a group's access to a workspace. Registry and admin grants live in Casbin only.
type GroupPermission struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	GroupID     uuid.UUID      `gorm:"type:text;not null;index" json:"group_id"`
	Group       Group          `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	WorkspaceID uuid.UUID      `gorm:"type:text;not null;index" json:"workspace_id"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	RoleID      uint           `gorm:"not null" json:"role_id"`
	Role        Role           `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
