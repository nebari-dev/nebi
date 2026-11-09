package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VersionPublication is publication metadata for a project version stored on
// the team server (issue #649, gap 2). Clients push it alongside versions so
// teammates see the "Published" badge (registry/repository:tag) after a
// pull. It never holds credentials and does not reference oci_registries:
// the registry a client published to usually is not configured on the server.
type VersionPublication struct {
	ID            uuid.UUID `gorm:"type:text;primary_key" json:"id"`
	ProjectID     uuid.UUID `gorm:"type:text;not null;index:idx_version_publication" json:"project_id"`
	VersionNumber int       `gorm:"not null;index:idx_version_publication" json:"version_number"`
	RegistryHost  string    `gorm:"not null" json:"registry_host"` // e.g. ghcr.io
	Repository    string    `gorm:"not null" json:"repository"`    // e.g. example-org/project_1
	Tag           string    `gorm:"not null" json:"tag"`
	Digest        string    `json:"digest,omitempty"`
	PublishedBy   string    `json:"published_by,omitempty"` // display name snapshot
	PublishedAt   time.Time `json:"published_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	// UnpublishedAt is set when a client reports the tag was removed.
	UnpublishedAt *time.Time `json:"unpublished_at,omitempty"`
}

// TableName pins the table name.
func (VersionPublication) TableName() string { return "version_publications" }

// BeforeCreate generates the UUID.
func (p *VersionPublication) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
