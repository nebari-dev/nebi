package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OCIRegistry represents an OCI-compliant container registry
type OCIRegistry struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	Name      string     `gorm:"uniqueIndex;not null" json:"name"` // e.g., "GitHub Container Registry"
	URL       string     `gorm:"not null" json:"url"`              // e.g., "ghcr.io"
	Username  string     `json:"username"`
	Password  string     `json:"-"` // encrypted, never exposed in JSON
	IsDefault bool       `gorm:"default:false" json:"is_default"`
	CreatedBy uuid.UUID  `gorm:"type:uuid" json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
}

// Publication tracks when and where an environment was published
type Publication struct {
	ID            uuid.UUID   `gorm:"type:uuid;primary_key" json:"id"`
	EnvironmentID uuid.UUID   `gorm:"type:uuid;index;not null" json:"environment_id"`
	Environment   Environment `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
	RegistryID    uuid.UUID   `gorm:"type:uuid;index;not null" json:"registry_id"`
	Registry      OCIRegistry `gorm:"foreignKey:RegistryID" json:"registry,omitempty"`
	Repository    string      `gorm:"not null" json:"repository"` // e.g., "myorg/myenv"
	Tag           string      `gorm:"not null" json:"tag"`        // e.g., "v1.0.0"
	Digest        string      `json:"digest"`                     // OCI manifest digest
	PublishedBy   uuid.UUID   `gorm:"type:uuid;not null" json:"published_by"`
	PublishedByUser User      `gorm:"foreignKey:PublishedBy" json:"published_by_user,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	DeletedAt     *time.Time  `gorm:"index" json:"-"`
}

// TableName specifies the table name for OCIRegistry
func (OCIRegistry) TableName() string {
	return "oci_registries"
}

// TableName specifies the table name for Publication
func (Publication) TableName() string {
	return "publications"
}

// BeforeCreate will set a UUID rather than numeric ID
func (r *OCIRegistry) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// BeforeCreate will set a UUID rather than numeric ID
func (p *Publication) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
