package cliclient

import "time"

// LoginRequest represents a login request.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response.
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// User represents a user.
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Workspace represents a workspace.
type Workspace struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	PackageManager string    `json:"package_manager"`
	SizeBytes      int64     `json:"size_bytes"`
	Owner          *User     `json:"owner,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateWorkspaceRequest represents a request to create a workspace.
type CreateWorkspaceRequest struct {
	Name           string  `json:"name"`
	PackageManager *string `json:"package_manager,omitempty"`
	PixiToml       *string `json:"pixi_toml,omitempty"`
}

// Package represents a package in a workspace.
type Package struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Source    string `json:"source,omitempty"`
	WsID      string `json:"workspace_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Publication represents a published version of a workspace.
type Publication struct {
	ID            string `json:"id"`
	WsID          string `json:"workspace_id"`
	VersionNumber int    `json:"version_number"`
	RegistryID    string `json:"registry_id"`
	RegistryName  string `json:"registry_name"`
	Repository    string `json:"repository"`
	Tag           string `json:"tag"`
	Digest        string `json:"digest"`
	PublishedAt   string `json:"published_at"`
}

// PublishDefaults represents suggested defaults for publishing a workspace.
type PublishDefaults struct {
	RegistryID   string `json:"registry_id"`
	RegistryName string `json:"registry_name"`
	Namespace    string `json:"namespace"`
	Repository   string `json:"repository"`
	Tag          string `json:"tag"`
}

// PublishRequest represents a request to publish a workspace.
type PublishRequest struct {
	RegistryID string `json:"registry_id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

// PublishResponse represents the response from publishing a workspace.
type PublishResponse struct {
	Digest     string `json:"digest"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

// WorkspaceVersion represents a version of a workspace.
type WorkspaceVersion struct {
	ID            string `json:"id"`
	WsID          string `json:"workspace_id"`
	VersionNumber int32  `json:"version_number"`
	CreatedAt     string `json:"created_at"`
}

// Registry represents an OCI registry.
type Registry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Username  string `json:"username,omitempty"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CreateRegistryRequest represents a request to create a registry.
type CreateRegistryRequest struct {
	Name      string  `json:"name"`
	URL       string  `json:"url"`
	Username  *string `json:"username,omitempty"`
	Password  *string `json:"password,omitempty"`
	IsDefault *bool   `json:"is_default,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
}

// UpdateRegistryRequest represents a request to update a registry.
type UpdateRegistryRequest struct {
	Name      *string `json:"name,omitempty"`
	URL       *string `json:"url,omitempty"`
	Username  *string `json:"username,omitempty"`
	Password  *string `json:"password,omitempty"`
	IsDefault *bool   `json:"is_default,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
}

// PushRequest represents a request to push a version to the server.
type PushRequest struct {
	Tag      string `json:"tag"`
	PixiToml string `json:"pixi_toml"`
	PixiLock string `json:"pixi_lock,omitempty"`
	Force    bool   `json:"force,omitempty"`
}

// PushResponse represents the response from pushing a version.
type PushResponse struct {
	VersionNumber int    `json:"version_number"`
	Tag           string `json:"tag"`
}

// WorkspaceTag represents a server-side tag pointing to a version.
type WorkspaceTag struct {
	Tag           string `json:"tag"`
	VersionNumber int    `json:"version_number"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// Job represents a background job on the server.
type Job struct {
	ID          string                 `json:"id"`
	WorkspaceID string                 `json:"workspace_id"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Logs        string                 `json:"logs,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	StartedAt   *string                `json:"started_at,omitempty"`
	CompletedAt *string                `json:"completed_at,omitempty"`
}

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID          int         `json:"id"`
	UserID      string      `json:"user_id"`
	Action      string      `json:"action"`
	Resource    string      `json:"resource"`
	ResourceID  string      `json:"resource_id,omitempty"`
	DetailsJSON interface{} `json:"details_json,omitempty"`
	Timestamp   string      `json:"timestamp"`
	User        *User       `json:"user,omitempty"`
}

// DashboardStats represents admin dashboard statistics.
type DashboardStats struct {
	TotalDiskUsageBytes     int64  `json:"total_disk_usage_bytes"`
	TotalDiskUsageFormatted string `json:"total_disk_usage_formatted"`
}
