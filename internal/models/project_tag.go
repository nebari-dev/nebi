package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectTag represents a named pointer to a specific version of a project.
// Tags are mutable — pushing the same tag again re-points it to the new version.
type ProjectTag struct {
	ID            uuid.UUID `gorm:"type:text;primary_key" json:"id"`
	ProjectID     uuid.UUID `gorm:"type:text;not null;uniqueIndex:idx_project_tag" json:"project_id"`
	Project       *Project  `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Tag           string    `gorm:"not null;uniqueIndex:idx_project_tag" json:"tag"`
	VersionNumber int       `gorm:"not null" json:"version_number"`
	CreatedBy     uuid.UUID `gorm:"type:text;not null" json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BeforeCreate hook to generate UUID
func (wt *ProjectTag) BeforeCreate(tx *gorm.DB) error {
	if wt.ID == uuid.Nil {
		wt.ID = uuid.New()
	}
	return nil
}
