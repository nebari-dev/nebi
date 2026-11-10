package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RemoteKind identifies what a ProjectRemote points at.
type RemoteKind string

const (
	// RemoteKindServer links a local project to a project on the connected
	// Nebi team server (push, pull, sync markers, update available, share).
	RemoteKindServer RemoteKind = "server"
	// RemoteKindRegistry records an OCI registry repository the project was
	// pulled from or published to.
	RemoteKindRegistry RemoteKind = "registry"
)

// ProjectRemote is a "connected remote" of a local project (local mode only,
// issue #649). The projects list renders them as the Remotes column. A
// project has at most one server remote; it can have several registry
// remotes. Disconnecting from the server keeps these rows so pulled projects
// keep working and can re-link on reconnect.
type ProjectRemote struct {
	ID        uuid.UUID  `gorm:"type:text;primary_key" json:"id"`
	ProjectID uuid.UUID  `gorm:"type:text;not null;index" json:"project_id"`
	Kind      RemoteKind `gorm:"not null" json:"kind"`
	// Name is the display label, e.g. "team-nebi" or "ghcr.io/example-org".
	Name string `gorm:"not null" json:"name"`

	// Server remote fields.
	ServerURL         string `json:"server_url,omitempty"`
	ServerProjectID   string `json:"server_project_id,omitempty"`
	ServerProjectName string `json:"server_project_name,omitempty"`

	// Registry remote fields. RegistryID references oci_registries in the
	// local database when the registry is configured locally.
	RegistryID   *uuid.UUID `gorm:"type:text" json:"registry_id,omitempty"`
	RegistryHost string     `json:"registry_host,omitempty"`
	Repository   string     `json:"repository,omitempty"`

	// LastSyncedAt is the last time server state for this project was fetched.
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name.
func (ProjectRemote) TableName() string { return "project_remotes" }

// BeforeCreate generates the UUID.
func (r *ProjectRemote) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
