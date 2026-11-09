package core

import (
	"time"

	"github.com/google/uuid"
)

// JSON bodies of the team-server endpoints added for issue #649. They are
// the wire format internal/client/sdk/remote speaks; the client mirrors
// them in its own types (the server never imports client packages).

// CatalogProject is a server project visible to the caller.
// GET /api/v1/catalog/projects
type CatalogProject struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Owner         string    `json:"owner"`     // owner username
	Namespace     string    `json:"namespace"` // "Personal" or a team name
	LatestVersion *int      `json:"latest_version"`
	UpdatedAt     time.Time `json:"updated_at"`
	Role          string    `json:"role"` // caller's role: owner | edit | view
}

// CreateCatalogProjectRequest creates an empty server project (first push).
type CreateCatalogProjectRequest struct {
	Name string `json:"name"`
}

// SyncVersion is a server version without file contents.
// GET /api/v1/projects/:id/sync/versions
type SyncVersion struct {
	Number       int                 `json:"number"`
	Description  string              `json:"description"`
	Author       string              `json:"author"`
	CreatedAt    time.Time           `json:"created_at"`
	ContentHash  string              `json:"content_hash"`
	BaseVersion  *int                `json:"base_version"`
	Tags         []string            `json:"tags"`
	Publications []PublicationRecord `json:"publications"`
}

// SyncVersionFiles is a server version with its files.
// GET /api/v1/projects/:id/sync/versions/:version
type SyncVersionFiles struct {
	SyncVersion
	PixiToml string `json:"pixi_toml"`
	PixiLock string `json:"pixi_lock"`
}

// PushVersion is one version a client pushes. Number is the client's
// version number; the server stores it as is (numbers are shared between
// a server project and the machines that pull it).
type PushVersion struct {
	Number       int                 `json:"number"`
	BaseVersion  *int                `json:"base_version"`
	Description  string              `json:"description"`
	Author       string              `json:"author"`
	PixiToml     string              `json:"pixi_toml"`
	PixiLock     string              `json:"pixi_lock"`
	Tags         []string            `json:"tags"`
	Publications []PublicationRecord `json:"publications"`
}

// PushVersionsRequest pushes versions in ascending order.
// POST /api/v1/projects/:id/sync/versions
type PushVersionsRequest struct {
	Versions []PushVersion `json:"versions"`
}

// PushVersionsResult reports what the server stored.
type PushVersionsResult struct {
	Accepted      []int `json:"accepted"`
	LatestVersion int   `json:"latest_version"`
}

// PublicationRecord is publication metadata (never credentials).
type PublicationRecord struct {
	ID           *uuid.UUID `json:"id,omitempty"`
	RegistryHost string     `json:"registry_host"`
	Repository   string     `json:"repository"`
	Tag          string     `json:"tag"`
	Digest       string     `json:"digest,omitempty"`
	PublishedBy  string     `json:"published_by,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
}

// Principal is a user or a group. GET /api/v1/principals?q=
type Principal struct {
	Kind    string `json:"kind"` // "user" | "group"
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Members int    `json:"members,omitempty"`
}

// AccessEntry is one principal with its role (owner | edit | view).
type AccessEntry struct {
	Principal Principal `json:"principal"`
	Role      string    `json:"role"`
}

// AccessList is GET /api/v1/projects/:id/access.
type AccessList struct {
	Entries   []AccessEntry `json:"entries"`
	CanManage bool          `json:"can_manage"`
}

// PrincipalRef identifies a principal.
type PrincipalRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// GrantAccessRequest is POST /api/v1/projects/:id/access.
type GrantAccessRequest struct {
	Principals []PrincipalRef `json:"principals"`
	Role       string         `json:"role"` // edit | view
}

// UpdateAccessRequest is PUT /api/v1/projects/:id/access/:kind/:principalId.
type UpdateAccessRequest struct {
	Role string `json:"role"` // edit | view
}
