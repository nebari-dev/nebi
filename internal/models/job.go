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
	JobTypeInstall  JobType = "install"
	JobTypeRemove   JobType = "remove"
	JobTypeUpdate   JobType = "update"
	JobTypeRollback JobType = "rollback"
)

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
	ID            uuid.UUID              `gorm:"type:text;primary_key" json:"id"`
	WorkspaceID uuid.UUID              `gorm:"type:text;index" json:"workspace_id"`
	Workspace   Workspace              `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	Type          JobType                `gorm:"not null" json:"type"`
	Status        JobStatus              `gorm:"not null;default:'pending'" json:"status"`
	Logs          string                 `gorm:"type:text" json:"logs"`
	Error         string                 `gorm:"type:text" json:"error,omitempty"`
	Metadata      map[string]interface{} `gorm:"serializer:json" json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	StartedAt     *time.Time             `json:"started_at,omitempty"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	DeletedAt     gorm.DeletedAt         `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (j *Job) BeforeCreate(tx *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return nil
}
