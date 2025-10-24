package models

import (
	"time"

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
	ID             uint              `gorm:"primarykey" json:"id"`
	Name           string            `gorm:"not null" json:"name"`
	OwnerID        uint              `gorm:"not null;index" json:"owner_id"`
	Owner          User              `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Status         EnvironmentStatus `gorm:"not null;default:'pending'" json:"status"`
	PackageManager string            `gorm:"not null" json:"package_manager"` // "pixi" or "uv"
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	DeletedAt      gorm.DeletedAt    `gorm:"index" json:"-"`
}
