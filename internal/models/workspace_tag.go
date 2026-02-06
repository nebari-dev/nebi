package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkspaceTag represents a named pointer to a specific version of a workspace.
// Tags are mutable — pushing the same tag again re-points it to the new version.
type WorkspaceTag struct {
	ID          uuid.UUID  `gorm:"type:text;primary_key" json:"id"`
	WorkspaceID uuid.UUID  `gorm:"type:text;not null;uniqueIndex:idx_ws_tag" json:"workspace_id"`
	Workspace   *Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	Tag         string     `gorm:"not null;uniqueIndex:idx_ws_tag" json:"tag"`
	VersionNumber int      `gorm:"not null" json:"version_number"`
	CreatedBy   uuid.UUID  `gorm:"type:text;not null" json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// BeforeCreate hook to generate UUID
func (wt *WorkspaceTag) BeforeCreate(tx *gorm.DB) error {
	if wt.ID == uuid.Nil {
		wt.ID = uuid.New()
	}
	return nil
}
