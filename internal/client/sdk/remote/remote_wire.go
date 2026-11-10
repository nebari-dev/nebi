package remote

// Owner: U5 (remote). Wire types of the team-server endpoints added for
// issue #649 (internal/server/core/types.go). The client keeps its own
// copy so it never imports server packages.

import (
	"time"
)

// catalogProject is GET /api/v1/catalog/projects.
type catalogProject struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Owner         string    `json:"owner"`
	Namespace     string    `json:"namespace"`
	LatestVersion *int      `json:"latest_version"`
	UpdatedAt     time.Time `json:"updated_at"`
	Role          string    `json:"role"`
}

// syncVersion is GET /api/v1/projects/:id/sync/versions (oldest first).
type syncVersion struct {
	Number       int                 `json:"number"`
	Description  string              `json:"description"`
	Author       string              `json:"author"`
	CreatedAt    time.Time           `json:"created_at"`
	ContentHash  string              `json:"content_hash"`
	BaseVersion  *int                `json:"base_version"`
	Tags         []string            `json:"tags"`
	Publications []publicationRecord `json:"publications"`
}

// syncVersionFiles is GET /api/v1/projects/:id/sync/versions/:version.
type syncVersionFiles struct {
	syncVersion
	PixiToml string `json:"pixi_toml"`
	PixiLock string `json:"pixi_lock"`
}

// pushVersion is one entry of POST /api/v1/projects/:id/sync/versions.
type pushVersion struct {
	Number       int                 `json:"number"`
	BaseVersion  *int                `json:"base_version"`
	Description  string              `json:"description"`
	Author       string              `json:"author"`
	PixiToml     string              `json:"pixi_toml"`
	PixiLock     string              `json:"pixi_lock"`
	Tags         []string            `json:"tags"`
	Publications []publicationRecord `json:"publications"`
}

type pushVersionsRequest struct {
	Versions []pushVersion `json:"versions"`
}

type pushVersionsResult struct {
	Accepted      []int `json:"accepted"`
	LatestVersion int   `json:"latest_version"`
}

// publicationRecord is publication metadata (never credentials).
type publicationRecord struct {
	ID           *string    `json:"id,omitempty"`
	RegistryHost string     `json:"registry_host"`
	Repository   string     `json:"repository"`
	Tag          string     `json:"tag"`
	Digest       string     `json:"digest,omitempty"`
	PublishedBy  string     `json:"published_by,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
}

// serverUser is the subset of GET /api/v1/auth/me the client needs.
type serverUser struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// serverVersionInfo is the subset of GET /api/v1/version the client needs.
type serverVersionInfo struct {
	Version string `json:"version"`
	Mode    string `json:"mode"`
}
