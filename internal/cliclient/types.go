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

// Environment represents an environment/workspace.
type Environment struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	PackageManager string    `json:"package_manager"`
	SizeBytes      int64     `json:"size_bytes"`
	Owner          *User     `json:"owner,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateEnvironmentRequest represents a request to create an environment.
type CreateEnvironmentRequest struct {
	Name           string  `json:"name"`
	PackageManager *string `json:"package_manager,omitempty"`
	PixiToml       *string `json:"pixi_toml,omitempty"`
}

// Package represents a package in an environment.
type Package struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Source    string `json:"source,omitempty"`
	EnvID     string `json:"environment_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Publication represents a published version of an environment.
type Publication struct {
	ID            string `json:"id"`
	EnvID         string `json:"environment_id"`
	VersionNumber int    `json:"version_number"`
	RegistryID    string `json:"registry_id"`
	RegistryName  string `json:"registry_name"`
	Repository    string `json:"repository"`
	Tag           string `json:"tag"`
	Digest        string `json:"digest"`
	PublishedAt   string `json:"published_at"`
}

// PublishRequest represents a request to publish an environment.
type PublishRequest struct {
	RegistryID string `json:"registry_id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

// PublishResponse represents the response from publishing an environment.
type PublishResponse struct {
	Digest     string `json:"digest"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

// EnvironmentVersion represents a version of an environment.
type EnvironmentVersion struct {
	ID            string `json:"id"`
	EnvID         string `json:"environment_id"`
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
}

// UpdateRegistryRequest represents a request to update a registry.
type UpdateRegistryRequest struct {
	Name      *string `json:"name,omitempty"`
	URL       *string `json:"url,omitempty"`
	Username  *string `json:"username,omitempty"`
	Password  *string `json:"password,omitempty"`
	IsDefault *bool   `json:"is_default,omitempty"`
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

// EnvironmentTag represents a server-side tag pointing to a version.
type EnvironmentTag struct {
	Tag           string `json:"tag"`
	VersionNumber int    `json:"version_number"`
}
