package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectStatus represents the state of a project
type ProjectStatus string

const (
	ProjectStatusPending  ProjectStatus = "pending"
	ProjectStatusCreating ProjectStatus = "creating"
	ProjectStatusReady    ProjectStatus = "ready"
	ProjectStatusFailed   ProjectStatus = "failed"
	ProjectStatusDeleting ProjectStatus = "deleting"
)

// IsTransitional reports whether the project is mid-transition
// (pending/creating/deleting) and therefore cannot accept jobs. Ready and
// failed projects are settled: a failed project may accept new jobs so a
// corrected spec or a rollback can recover it (issue #497).
func (w ProjectStatus) IsTransitional() bool {
	return w != ProjectStatusReady && w != ProjectStatusFailed
}

// InstallStatus describes whether a project's environment is
// materialized on disk (.pixi/envs). It is derived from disk state and
// env job state, never stored in the database. Local mode only.
type InstallStatus string

const (
	InstallStatusNotInstalled InstallStatus = "not_installed"
	InstallStatusInstalling   InstallStatus = "installing"
	InstallStatusInstalled    InstallStatus = "installed"
	InstallStatusUninstalling InstallStatus = "uninstalling"
	InstallStatusFailed       InstallStatus = "install_failed"
)

// Project represents a Nebi project backed by a Pixi workspace.
type Project struct {
	ID        uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Name      string         `gorm:"not null" json:"name"`
	OwnerID   uuid.UUID      `gorm:"type:text;index" json:"owner_id"`
	Owner     User           `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Status    ProjectStatus  `gorm:"not null;default:'pending'" json:"status"`
	Source    string         `gorm:"default:'managed'" json:"source"` // "managed", "local"
	Path      string         `json:"path,omitempty"`                  // filesystem path (local-mode)
	SizeBytes int64          `gorm:"default:0" json:"size_bytes,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName ensures GORM uses the "projects" table
func (Project) TableName() string {
	return "projects"
}

// BeforeCreate hook to generate UUID
func (w *Project) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}
