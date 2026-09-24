package service

import (
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/utils"
)

// CreateRequest holds parameters for creating a project.
type CreateRequest struct {
	Name             string
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

// ProjectResponse wraps a project with computed fields.
// InstallStatus and size fields are populated in local mode only; the
// server no longer installs environments, so team-mode responses omit them.
type ProjectResponse struct {
	models.Project
	SizeFormatted string               `json:"size_formatted,omitempty"`
	InstallStatus models.InstallStatus `json:"install_status,omitempty"`
}

// ProjectDetailResponse includes the current user's effective write access.
type ProjectDetailResponse struct {
	ProjectResponse
	CanWrite bool `json:"can_write"`
}

// NewProjectResponse creates a ProjectResponse with formatted size.
func NewProjectResponse(project models.Project) ProjectResponse {
	resp := ProjectResponse{Project: project}
	if project.SizeBytes > 0 {
		resp.SizeFormatted = utils.FormatBytes(project.SizeBytes)
	}
	return resp
}

// CollaboratorKind identifies whether a collaborator entry is a user or a group.
type CollaboratorKind string

const (
	CollaboratorKindUser  CollaboratorKind = "user"
	CollaboratorKindGroup CollaboratorKind = "group"
)

// CollaboratorResult is the result type for ListCollaborators.
type CollaboratorResult struct {
	Kind     CollaboratorKind `json:"kind"`
	UserID   *uuid.UUID       `json:"user_id,omitempty"`
	Username string           `json:"username,omitempty"`
	Email    string           `json:"email,omitempty"`
	GroupID  *uuid.UUID       `json:"group_id,omitempty"`
	Name     string           `json:"name,omitempty"`
	Source   string           `json:"source,omitempty"` // "" for users, "native"/"oidc" for groups
	Role     string           `json:"role"`
	IsOwner  bool             `json:"is_owner"`
}

// PublishProjectRequest holds parameters for publishing to an OCI registry.
type PublishProjectRequest struct {
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
