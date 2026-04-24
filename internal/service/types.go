package service

import (
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/utils"
)

// CreateRequest holds parameters for creating a workspace.
type CreateRequest struct {
	Name             string
	PackageManager   string
	PixiToml         string
	Source           string
	Path             string
	ImportStagingDir string // absolute path to a pre-extracted bundle directory; worker hands it to the executor as SeedDir
}

// PushRequest holds parameters for pushing a new version.
type PushRequest struct {
	Tag      string
	PixiToml string
	PixiLock string
	Force    bool
}

// PushResult is returned after a successful push.
type PushResult struct {
	VersionNumber int
	Tags          []string
	ContentHash   string
	Deduplicated  bool
	Tag           string // kept for backwards compatibility
}

// WorkspaceResponse wraps a workspace with computed fields.
type WorkspaceResponse struct {
	models.Workspace
	SizeFormatted string `json:"size_formatted"`
}

// NewWorkspaceResponse creates a WorkspaceResponse with formatted size.
func NewWorkspaceResponse(ws models.Workspace) WorkspaceResponse {
	return WorkspaceResponse{
		Workspace:     ws,
		SizeFormatted: utils.FormatBytes(ws.SizeBytes),
	}
}

// CollaboratorResult is the result type for ListCollaborators.
type CollaboratorResult struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	Email    string    `json:"email,omitempty"`
	Role     string    `json:"role"`
	IsOwner  bool      `json:"is_owner"`
}

// PublishWorkspaceRequest holds parameters for publishing to an OCI registry.
type PublishWorkspaceRequest struct {
	RegistryID uuid.UUID
	Repository string
	Tag        string
}

// PublicationResult is the denormalized publication info ready for JSON.
type PublicationResult struct {
	ID                uuid.UUID `json:"id"`
	VersionNumber     int       `json:"version_number"`
	RegistryName      string    `json:"registry_name"`
	RegistryURL       string    `json:"registry_url"`
	RegistryNamespace string    `json:"registry_namespace"`
	Repository        string    `json:"repository"`
	Tag               string    `json:"tag"`
	Digest            string    `json:"digest"`
	IsPublic          bool      `json:"is_public"`
	PublishedBy       string    `json:"published_by"`
	PublishedAt       string    `json:"published_at"`
}

// PublishDefaultsResult holds precomputed defaults for the publish dialog.
type PublishDefaultsResult struct {
	RegistryID   uuid.UUID `json:"registry_id"`
	RegistryName string    `json:"registry_name"`
	Namespace    string    `json:"namespace"`
	Repository   string    `json:"repository"`
	Tag          string    `json:"tag"`
}
