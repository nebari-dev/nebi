package models

import (
	"time"

	"gorm.io/gorm"
)

// Package represents an installed package in an environment
type Package struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	EnvironmentID uint           `gorm:"not null;index" json:"environment_id"`
	Environment   Environment    `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	Name          string         `gorm:"not null" json:"name"`
	Version       string         `json:"version"`
	InstalledAt   time.Time      `json:"installed_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
