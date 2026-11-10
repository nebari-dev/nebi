package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// JobType represents the type of job
type JobType string

const (
	JobTypeCreate   JobType = "create"
	JobTypeDelete   JobType = "delete"
	JobTypeInstall  JobType = "install" // add packages to the manifest
	JobTypeRemove   JobType = "remove"
	JobTypeUpdate   JobType = "update"
	JobTypeRollback JobType = "rollback"
	// JobTypeEnvInstall materializes the environment from the lockfile
	// (pixi install); JobTypeEnvUninstall removes .pixi/envs. Both are
	// local-mode operations.
	JobTypeEnvInstall   JobType = "env_install"
	JobTypeEnvUninstall JobType = "env_uninstall"

	// Version-model job types (issue #649). Every change is a new version
	// and one version is installed at a time. "create" (JobTypeCreate) still
	// creates a project and its version 1; with metadata install=true it
	// also installs it ("Create and install"). "delete" (JobTypeDelete)
	// still deletes a project. The edit-in-place types above (install,
	// remove, update, rollback) remain only for the legacy API.
	JobTypeCreateVersion  JobType = "create_version"  // solve a new version from a base version + edited pixi.toml
	JobTypeInstallVersion JobType = "install_version" // install a specific version, replacing the installed one
	JobTypeUninstall      JobType = "uninstall"       // remove the installed environment
	JobTypePull           JobType = "pull"            // pull a server project without installing
	JobTypePullInstall    JobType = "pull_install"    // pull a server project and install its latest version
	JobTypePullRegistry   JobType = "pull_registry"   // pull a project from an OCI registry
)

// JobTypeLabels are the user-facing names for job types (Jobs page filter,
// job detail title, toasts). Keep in sync with JOB_TYPE_LABELS in
// frontend/apps/client/src/types/contracts.ts.
var JobTypeLabels = map[JobType]string{
	JobTypeCreate:         "Create project",
	JobTypeDelete:         "Delete project",
	JobTypeCreateVersion:  "Create version",
	JobTypeInstallVersion: "Install version",
	JobTypeUninstall:      "Uninstall",
	JobTypePull:           "Pull project",
	JobTypePullInstall:    "Pull and install",
	JobTypePullRegistry:   "Pull from registry",
	JobTypeInstall:        "Add packages",
	JobTypeRemove:         "Remove packages",
	JobTypeUpdate:         "Update project",
	JobTypeRollback:       "Rollback",
	JobTypeEnvInstall:     "Install environment",
	JobTypeEnvUninstall:   "Uninstall environment",
}

// JobStatus represents the state of a job
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// Job represents a background task
type Job struct {
	ID        uuid.UUID              `gorm:"type:text;primary_key" json:"id"`
	ProjectID uuid.UUID              `gorm:"type:text;index" json:"project_id"`
	Project   Project                `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	UserID    uuid.UUID              `gorm:"type:text;index" json:"user_id,omitempty"`
	Type      JobType                `gorm:"not null" json:"type"`
	Status    JobStatus              `gorm:"not null;default:'pending'" json:"status"`
	Logs      string                 `gorm:"type:text" json:"logs"`
	Error     string                 `gorm:"type:text" json:"error,omitempty"`
	Metadata  map[string]interface{} `gorm:"serializer:json" json:"metadata,omitempty"`
	// VersionNumber links the job to the project version it created,
	// installed, pulled or published (nil when not applicable).
	VersionNumber *int           `json:"version_number,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (j *Job) BeforeCreate(tx *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return nil
}
