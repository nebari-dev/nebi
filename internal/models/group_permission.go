package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupPermission represents group-level access to a workspace.
// Groups are IdP-managed (synced from Keycloak), not managed within Nebi.
type GroupPermission struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	GroupName   string         `gorm:"not null;index" json:"group_name"`
	WorkspaceID uuid.UUID      `gorm:"type:text;not null;index" json:"workspace_id"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	RoleID      uint           `gorm:"not null" json:"role_id"`
	Role        Role           `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
