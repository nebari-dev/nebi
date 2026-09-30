package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Package represents an installed package in a project
type Package struct {
	ID          uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	ProjectID   uuid.UUID      `gorm:"type:text;not null;index" json:"project_id"`
	Project     Project        `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Name        string         `gorm:"not null" json:"name"`
	Version     string         `json:"version"`
	InstalledAt time.Time      `json:"installed_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (p *Package) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
