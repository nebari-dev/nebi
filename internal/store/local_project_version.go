package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LocalProjectVersion mirrors models.ProjectVersion for CLI/local use.
// It shares the "project_versions" table with the server-side model so the
// GUI (which uses models.ProjectVersion) and the CLI can both read/write
// versions in local mode.
type LocalProjectVersion struct {
	ID            uuid.UUID `gorm:"type:text;primary_key" json:"id"`
	ProjectID     uuid.UUID `gorm:"type:text;not null;index:idx_project_version" json:"project_id"`
	VersionNumber int       `gorm:"not null;index:idx_project_version" json:"version_number"`

	LockFileContent string `gorm:"type:text;not null" json:"lock_file_content"`
	ManifestContent string `gorm:"type:text;not null" json:"manifest_content"`
	PackageMetadata string `gorm:"type:text;not null" json:"package_metadata"`

	ContentHash string `gorm:"type:text;index" json:"content_hash"`

	JobID       *uuid.UUID `gorm:"type:text;index" json:"job_id,omitempty"`
	CreatedBy   uuid.UUID  `gorm:"type:text;not null" json:"created_by"`
	Description string     `gorm:"type:text" json:"description,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName ensures GORM uses the "project_versions" table.
func (LocalProjectVersion) TableName() string {
	return "project_versions"
}

// BeforeCreate generates the UUID and auto-increments the version number
// on a per-project basis. Matches models.ProjectVersion.BeforeCreate.
func (wv *LocalProjectVersion) BeforeCreate(tx *gorm.DB) error {
	if wv.ID == uuid.Nil {
		wv.ID = uuid.New()
	}

	if wv.VersionNumber == 0 {
		var maxVersion struct {
			MaxVersion *int
		}
		tx.Model(&LocalProjectVersion{}).
			Select("MAX(version_number) as max_version").
			Where("project_id = ?", wv.ProjectID).
			Scan(&maxVersion)

		if maxVersion.MaxVersion == nil {
			wv.VersionNumber = 1
		} else {
			wv.VersionNumber = *maxVersion.MaxVersion + 1
		}
	}

	return nil
}
