package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkspaceVersion represents a snapshot of workspace files at a point in time
type WorkspaceVersion struct {
	ID          uuid.UUID  `gorm:"type:text;primary_key" json:"id"`
	WorkspaceID uuid.UUID  `gorm:"type:text;not null;index:idx_ws_version" json:"workspace_id"`
	Workspace   *Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`

	// Version tracking
	VersionNumber int `gorm:"not null;index:idx_ws_version" json:"version_number"` // Auto-incrementing per workspace

	// File contents (stored as TEXT in database)
	LockFileContent string `gorm:"type:text;not null" json:"lock_file_content"` // pixi.lock content
	ManifestContent string `gorm:"type:text;not null" json:"manifest_content"`  // pixi.toml content
	PackageMetadata string `gorm:"type:text;not null" json:"package_metadata"`  // JSON of package list

	// Content hash for deduplication
	ContentHash string `gorm:"type:text;index" json:"content_hash"`

	// Context
	JobID         *uuid.UUID `gorm:"type:text;index" json:"job_id,omitempty"` // Job that triggered this version
	Job           *Job       `gorm:"foreignKey:JobID" json:"job,omitempty"`
	CreatedBy     uuid.UUID  `gorm:"type:text;not null" json:"created_by"` // User who triggered the change
	CreatedByUser *User      `gorm:"foreignKey:CreatedBy" json:"created_by_user,omitempty"`
	Description   string     `gorm:"type:text" json:"description,omitempty"` // Optional description of changes

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID and version number
func (wv *WorkspaceVersion) BeforeCreate(tx *gorm.DB) error {
	if wv.ID == uuid.Nil {
		wv.ID = uuid.New()
	}

	// Auto-increment version number for this workspace
	if wv.VersionNumber == 0 {
		var maxVersion struct {
			MaxVersion *int
		}
		tx.Model(&WorkspaceVersion{}).
			Select("MAX(version_number) as max_version").
			Where("workspace_id = ?", wv.WorkspaceID).
			Scan(&maxVersion)

		if maxVersion.MaxVersion == nil {
			wv.VersionNumber = 1
		} else {
			wv.VersionNumber = *maxVersion.MaxVersion + 1
		}
	}

	return nil
}
