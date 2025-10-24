package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EnvironmentStatus represents the state of an environment
type EnvironmentStatus string

const (
	EnvStatusPending  EnvironmentStatus = "pending"
	EnvStatusCreating EnvironmentStatus = "creating"
	EnvStatusReady    EnvironmentStatus = "ready"
	EnvStatusFailed   EnvironmentStatus = "failed"
	EnvStatusDeleting EnvironmentStatus = "deleting"
)

// Environment represents a package manager environment
type Environment struct {
	ID             uuid.UUID         `gorm:"type:text;primary_key" json:"id"`
	Name           string            `gorm:"not null" json:"name"`
	OwnerID        uuid.UUID         `gorm:"type:text;not null;index" json:"owner_id"`
	Owner          User              `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Status         EnvironmentStatus `gorm:"not null;default:'pending'" json:"status"`
	PackageManager string            `gorm:"not null" json:"package_manager"` // "pixi" or "uv"
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	DeletedAt      gorm.DeletedAt    `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (e *Environment) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}
