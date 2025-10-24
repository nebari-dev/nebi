package models

import (
	"time"

	"gorm.io/gorm"
)

// JobType represents the type of job
type JobType string

const (
	JobTypeCreateEnv   JobType = "create_environment"
	JobTypeDeleteEnv   JobType = "delete_environment"
	JobTypeInstallPkg  JobType = "install_package"
	JobTypeRemovePkg   JobType = "remove_package"
	JobTypeUpdatePkg   JobType = "update_package"
)

// JobStatus represents the state of a job
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusRunning    JobStatus = "running"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
)

// Job represents a background task
type Job struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	EnvironmentID uint           `gorm:"index" json:"environment_id"`
	Environment   Environment    `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	Type          JobType        `gorm:"not null" json:"type"`
	Status        JobStatus      `gorm:"not null;default:'queued'" json:"status"`
	Logs          string         `gorm:"type:text" json:"logs"`
	Error         string         `gorm:"type:text" json:"error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
