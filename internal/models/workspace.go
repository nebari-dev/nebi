package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkspaceStatus represents the state of a workspace
type WorkspaceStatus string

const (
	WsStatusPending  WorkspaceStatus = "pending"
	WsStatusCreating WorkspaceStatus = "creating"
	WsStatusReady    WorkspaceStatus = "ready"
	WsStatusFailed   WorkspaceStatus = "failed"
	WsStatusDeleting WorkspaceStatus = "deleting"
)

// Workspace represents a package manager workspace
type Workspace struct {
	ID             uuid.UUID       `gorm:"type:text;primary_key" json:"id"`
	Name           string          `gorm:"not null" json:"name"`
	OwnerID        uuid.UUID       `gorm:"type:text;not null;index" json:"owner_id"`
	Owner          User            `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Status         WorkspaceStatus `gorm:"not null;default:'pending'" json:"status"`
	PackageManager string          `gorm:"not null" json:"package_manager"` // "pixi" or "uv"
	Source         string          `gorm:"default:'managed'" json:"source"` // "managed", "local"
	Path           string          `json:"path,omitempty"`                  // filesystem path (local-mode)
	SizeBytes      int64           `gorm:"default:0" json:"size_bytes"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	DeletedAt      gorm.DeletedAt  `gorm:"index" json:"-"`
}

// TableName ensures GORM uses the "workspaces" table
func (Workspace) TableName() string {
	return "workspaces"
}

// BeforeCreate hook to generate UUID
func (w *Workspace) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}
