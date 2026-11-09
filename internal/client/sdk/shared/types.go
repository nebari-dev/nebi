package shared

import (
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/models"
)

// The types below are the JSON bodies of the client REST API
// (/api/v1/client/...) and the values CLI commands print. Field comments
// name the unit that produces the field. Mirrored in
// frontend/apps/client/src/types/contracts.ts; keep both in sync.

// ---------------------------------------------------------------- manifest

// The manifest, spec, lock and diff shapes are internal/artifact's own JSON
// types (see CONTRACTS-artifact.md); the client API serves them as is.

// RequestedPackage is one requested dependency: Name, Constraint (a match
// spec such as ">=2.0", "3.11.*" or "*") and Source ("" or "conda", "pypi").
type RequestedPackage = artifact.RequestedPackage

// ManifestForm is the GUI form view of a pixi.toml: name, optional pixi
// description, channels, platforms and requested packages.
type ManifestForm = artifact.ManifestForm

// ManifestParseResult is a pixi.toml read into the form. Unrepresentable
// lists what the form cannot show (e.g. "[tasks]", "workspace.authors");
// switching to the form drops them ("You may lose some data...").
type ManifestParseResult struct {
	Form            ManifestForm `json:"form"`
	Unrepresentable []string     `json:"unrepresentable"`
}

// ManifestRenderRequest renders a form to pixi.toml. When BasePixiToml is
// set the form is applied onto it (artifact.ApplyForm) so tables the form
// cannot show survive; otherwise a fresh file is rendered.
type ManifestRenderRequest struct {
	Form         ManifestForm `json:"form"`
	BasePixiToml string       `json:"base_pixi_toml,omitempty"`
}

// ManifestRenderResult is the rendered pixi.toml.
type ManifestRenderResult struct {
	PixiToml string `json:"pixi_toml"`
}

// ----------------------------------------------------------------- projects

// RemoteRef is one "connected remote" shown in the projects list.
type RemoteRef struct {
	Kind            models.RemoteKind `json:"kind"`
	Name            string            `json:"name"`
	URL             string            `json:"url,omitempty"`
	ServerProjectID string            `json:"server_project_id,omitempty"`
}

// ProjectListItem is a row of the projects list (GET /client/projects) and
// the header of the project detail page.
type ProjectListItem struct {
	ID     uuid.UUID            `json:"id"`
	Name   string               `json:"name"`
	Path   string               `json:"path"`
	Status models.ProjectStatus `json:"status"`
	// InstallStatus is derived from disk + job state (U1, using exec).
	InstallStatus        models.InstallStatus `json:"install_status"`
	InstalledVersion     *int                 `json:"installed_version"`
	LastInstalledVersion *int                 `json:"last_installed_version"`
	LatestVersion        *int                 `json:"latest_version"`
	// SizeBytes is the installed environment size on disk; nil when not
	// installed or unknown. Size is the formatted value ("1.2 GB").
	SizeBytes *int64 `json:"size_bytes"`
	Size      string `json:"size"`
	// Remotes: local registry remotes from sdk/local (U1), server remote
	// filled by the sdk/remote decorator (U5). Never null.
	Remotes []RemoteRef `json:"remotes"`
	// UpdateAvailable: the linked server has versions not on this machine
	// (sdk/remote decorator, U5). ServerLatestVersion is that server's
	// newest version number.
	UpdateAvailable     bool `json:"update_available"`
	ServerLatestVersion *int `json:"server_latest_version"`
	// FailedInstallJobID is the most recent install job that failed and has
	// not been superseded by a successful install (jobs decorator, U8).
	FailedInstallJobID *uuid.UUID `json:"failed_install_job_id"`
	// ActiveJobID is a pending/running job on this project, if any (U1).
	ActiveJobID *uuid.UUID `json:"active_job_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateProjectRequest creates a project and its version 1. Exactly one of
// PixiToml (pixi.toml editor) or Form (GUI form) is set. Path empty means
// DefaultProjectsDir()/<name>.
type CreateProjectRequest struct {
	Name        string        `json:"name"`
	Path        string        `json:"path,omitempty"`
	PixiToml    string        `json:"pixi_toml,omitempty"`
	Form        *ManifestForm `json:"form,omitempty"`
	Description string        `json:"description,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	// Install also installs version 1 once created ("Create and install").
	Install bool `json:"install,omitempty"`
}

// CreateProjectResult is the new project (status "pending") and the create job.
type CreateProjectResult struct {
	Project ProjectListItem `json:"project"`
	Job     JobSummary      `json:"job"`
}

// NameCheckResult validates a project name: allowed characters and no
// duplicate among local projects. Error is user-facing copy.
type NameCheckResult struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

// ----------------------------------------------------------------- versions

// Author is who created a version.
type Author struct {
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

// PublishedRef is one "Published" badge entry: registry/repository:tag.
// Source "local" comes from this machine's publication records
// (sdk/registry, U7); "server" from the linked server (sdk/remote, U5).
type PublishedRef struct {
	PublicationID *uuid.UUID `json:"publication_id"`
	RegistryID    *uuid.UUID `json:"registry_id"`
	Registry      string     `json:"registry"`
	Repository    string     `json:"repository"`
	Tag           string     `json:"tag"`
	Reference     string     `json:"reference"` // registry/repository:tag
	Source        string     `json:"source"`
	// CanUnpublish is false when the registry cannot delete tags (gap 6)
	// or the publication was made elsewhere.
	CanUnpublish bool `json:"can_unpublish"`
}

// SyncMarker labels a version relative to the linked server.
type SyncMarker string

const (
	SyncNewestHere     SyncMarker = "newest_here"      // newest local version, not on the server yet
	SyncNewestOnServer SyncMarker = "newest_on_server" // newest server version
	SyncInSync         SyncMarker = "in_sync"          // newest here and newest on server are the same version
)

// VersionItem is a row of the version rail.
type VersionItem struct {
	Number      int       `json:"number"`
	Description string    `json:"description"`
	Author      Author    `json:"author"`
	CreatedAt   time.Time `json:"created_at"`
	// Tags excludes internal content-hash tags; "latest" is always on the
	// newest version.
	Tags        []string `json:"tags"`
	BaseVersion *int     `json:"base_version"`
	// Published: sdk/registry (local) + sdk/remote (server) decorators.
	Published []PublishedRef `json:"published"`
	// Sync and ServerOnly: sdk/remote decorator (U5). Server-only versions
	// exist only on the linked server and are appended by the decorator.
	Sync        *SyncMarker `json:"sync"`
	ServerOnly  bool        `json:"server_only"`
	Installed   bool        `json:"installed"`
	ContentHash string      `json:"content_hash,omitempty"`
}

// EnvironmentSpec is the "Environment specification" panel.
type EnvironmentSpec = artifact.Spec

// VersionDetail is one version with its environment spec.
type VersionDetail struct {
	VersionItem
	Spec EnvironmentSpec `json:"spec"`
}

// FileContent is a project file (pixi.toml or pixi.lock) of a version.
type FileContent struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ResolvedPackage is one package from pixi.lock (artifact.ResolvedPackage:
// name, version, build, source, platform, channel, channel_url, url, size).
type ResolvedPackage = artifact.ResolvedPackage

// ResolvedPackages are the default environment's resolved packages per
// platform, with the total download size per platform.
type ResolvedPackages struct {
	Platforms []string                     `json:"platforms"`
	Packages  map[string][]ResolvedPackage `json:"packages"`
	TotalSize map[string]int64             `json:"total_size"`
}

// InstallRequest installs a version; nil Version means the last installed
// version, else the latest ("Install" on the projects list).
type InstallRequest struct {
	Version *int `json:"version,omitempty"`
}

// CreateVersionRequest creates version N+1 from BaseVersion. Exactly one of
// PixiToml or Form is set. Tags move to the new version; "latest" always
// does (it is never sent).
type CreateVersionRequest struct {
	BaseVersion int           `json:"base_version"`
	PixiToml    string        `json:"pixi_toml,omitempty"`
	Form        *ManifestForm `json:"form,omitempty"`
	Description string        `json:"description"`
	Tags        []string      `json:"tags,omitempty"`
}

// ------------------------------------------------------------------ compare

// VersionDiff is artifact's version diff (requested and resolved rows with
// counts, channel and platform changes). Used by Compare versions and by
// the "Summary of changes" on the new version page.
type VersionDiff = artifact.VersionDiff

// CompareVersionRef labels a version in the compare pickers.
type CompareVersionRef struct {
	Number       int  `json:"number"`
	ServerOnly   bool `json:"server_only"`
	Installed    bool `json:"installed"`
	ServerLatest bool `json:"server_latest"`
}

// CompareResult is GET /client/projects/:id/compare: the diff fields are
// flattened next to base and target.
type CompareResult struct {
	Base   CompareVersionRef `json:"base"`
	Target CompareVersionRef `json:"target"`
	VersionDiff
}

// ------------------------------------------------------------------- remote

// ServerConnection is the single connected team server.
type ServerConnection struct {
	Connected bool   `json:"connected"`
	URL       string `json:"url"`
	// Name is the display label ("team-nebi"), derived from the host.
	Name         string     `json:"name"`
	Username     string     `json:"username"` // "Signed in as"
	Auth         string     `json:"auth"`     // "none" | "token"
	LastSyncedAt *time.Time `json:"last_synced_at"`
}

// ConnectRequest connects (or updates) the server connection.
type ConnectRequest struct {
	URL   string `json:"url"`
	Auth  string `json:"auth"` // "none" | "token"
	Token string `json:"token,omitempty"`
}

// ServerProjectItem is a project on the team server.
type ServerProjectItem struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Owner         string     `json:"owner"`
	Namespace     string     `json:"namespace"` // "Personal" or a team name
	LatestVersion *int       `json:"latest_version"`
	UpdatedAt     *time.Time `json:"updated_at"`
	Role          string     `json:"role"` // owner | edit | view (caller's role)
	// LocalProjectID is set when this machine already has the project.
	LocalProjectID *uuid.UUID `json:"local_project_id"`
}

// ServerProjectList is the Server page list.
type ServerProjectList struct {
	Projects     []ServerProjectItem `json:"projects"`
	LastSyncedAt *time.Time          `json:"last_synced_at"`
}

// PullRequest pulls a server project. LocalName defaults to the server name;
// a taken name returns ConflictError{Code: "name_taken"}.
type PullRequest struct {
	LocalName string `json:"local_name"`
	Path      string `json:"path,omitempty"`
	Install   bool   `json:"install"`
}

// PullResult is the new local project and its pull/pull_install job.
type PullResult struct {
	ProjectID uuid.UUID  `json:"project_id"`
	Job       JobSummary `json:"job"`
}

// PushResult reports which versions reached the server.
type PushResult struct {
	PushedVersions      []int  `json:"pushed_versions"`
	ServerLatestVersion int    `json:"server_latest_version"`
	ServerProjectID     string `json:"server_project_id"`
}

// RemoteDetails is the "Remote details" view of a server project.
type RemoteDetails struct {
	Project  ServerProjectItem `json:"project"`
	URL      string            `json:"url"`
	Versions []VersionItem     `json:"versions"`
}

// ------------------------------------------------------------------ sharing

// Principal is a user or a team (group) on the server.
type Principal struct {
	Kind    string `json:"kind"` // "user" | "group"
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Members int    `json:"members,omitempty"`
}

// AccessEntry is one row of "Who has access". Role: owner | edit | view.
type AccessEntry struct {
	Principal Principal `json:"principal"`
	Role      string    `json:"role"`
}

// AccessList is the share dialog content.
type AccessList struct {
	Entries   []AccessEntry `json:"entries"`
	CanManage bool          `json:"can_manage"`
}

// PrincipalRef identifies a principal in a grant.
type PrincipalRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// GrantAccessRequest adds people or teams with one role (edit | view).
type GrantAccessRequest struct {
	Principals []PrincipalRef `json:"principals"`
	Role       string         `json:"role"`
}

// UpdateAccessRequest changes a principal's role (edit | view).
type UpdateAccessRequest struct {
	Role string `json:"role"`
}

// ----------------------------------------------------------------- registry

// RegistryItem is a locally configured OCI registry. Passwords live in the
// OS keyring, never in SQLite or responses.
type RegistryItem struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Namespace      string    `json:"namespace"`
	Username       string    `json:"username"`
	HasCredentials bool      `json:"has_credentials"`
	IsDefault      bool      `json:"is_default"`
	// CanUnpublish: per-registry capability check for tag deletion (gap 6).
	CanUnpublish    bool   `json:"can_unpublish"`
	UnpublishReason string `json:"unpublish_reason,omitempty"`
}

// AddRegistryRequest adds a registry; Password goes to the keyring.
type AddRegistryRequest struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Namespace string `json:"namespace,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

// PublishTarget is one registry to publish to.
type PublishTarget struct {
	RegistryID uuid.UUID `json:"registry_id"`
	Repository string    `json:"repository"`
	Tag        string    `json:"tag"`
}

// PublishRequest publishes one version to several registries.
type PublishRequest struct {
	Targets []PublishTarget `json:"targets"`
}

// PublishDefault is the prefilled repository:tag for one registry.
type PublishDefault struct {
	RegistryID   uuid.UUID `json:"registry_id"`
	RegistryName string    `json:"registry_name"`
	Repository   string    `json:"repository"`
	Tag          string    `json:"tag"`
}

// PublishResultItem is one per-registry publish result.
type PublishResultItem struct {
	RegistryID    uuid.UUID  `json:"registry_id"`
	RegistryName  string     `json:"registry_name"`
	Reference     string     `json:"reference"`
	Status        string     `json:"status"` // "published" | "failed"
	Error         string     `json:"error,omitempty"`
	Digest        string     `json:"digest,omitempty"`
	PublicationID *uuid.UUID `json:"publication_id"`
}

// PublishResult is the per-registry outcome.
type PublishResult struct {
	Results []PublishResultItem `json:"results"`
}

// RegistryPullRequest pulls a project from a registry.
type RegistryPullRequest struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	LocalName  string `json:"local_name"`
	Install    bool   `json:"install"`
}

// --------------------------------------------------------------------- jobs

// JobSummary is a job row (Jobs page, toasts, notifications).
type JobSummary struct {
	ID            uuid.UUID        `json:"id"`
	Type          models.JobType   `json:"type"`
	TypeLabel     string           `json:"type_label"`
	Status        models.JobStatus `json:"status"`
	ProjectID     uuid.UUID        `json:"project_id"`
	ProjectName   string           `json:"project_name"`
	VersionNumber *int             `json:"version_number"`
	CreatedAt     time.Time        `json:"created_at"`
	StartedAt     *time.Time       `json:"started_at"`
	CompletedAt   *time.Time       `json:"completed_at"`
	DurationMs    *int64           `json:"duration_ms"`
	Error         string           `json:"error,omitempty"`
}

// JobDetail adds the persisted log.
type JobDetail struct {
	JobSummary
	Logs string `json:"logs"`
}

// JobListFilter filters GET /client/jobs. Sort is "started_at" or
// "-started_at" (default, newest first).
type JobListFilter struct {
	Statuses  []models.JobStatus
	Types     []models.JobType
	ProjectID *uuid.UUID
	Sort      string
	Limit     int
}

// JobSummaryFrom converts a job row.
func JobSummaryFrom(job models.Job, projectName string) JobSummary {
	s := JobSummary{
		ID:            job.ID,
		Type:          job.Type,
		TypeLabel:     models.JobTypeLabels[job.Type],
		Status:        job.Status,
		ProjectID:     job.ProjectID,
		ProjectName:   projectName,
		VersionNumber: job.VersionNumber,
		CreatedAt:     job.CreatedAt,
		StartedAt:     job.StartedAt,
		CompletedAt:   job.CompletedAt,
		Error:         job.Error,
	}
	if s.TypeLabel == "" {
		s.TypeLabel = string(job.Type)
	}
	if job.StartedAt != nil && job.CompletedAt != nil {
		d := job.CompletedAt.Sub(*job.StartedAt).Milliseconds()
		s.DurationMs = &d
	}
	return s
}

// ------------------------------------------------------------- capabilities

// Capabilities tells the UI what this host can do (GET /client/capabilities).
type Capabilities struct {
	// NativeFolderPicker is true in the desktop app only (gap 1).
	NativeFolderPicker bool   `json:"native_folder_picker"`
	Platform           string `json:"platform"` // GOOS/GOARCH, e.g. linux/amd64
	DefaultProjectsDir string `json:"default_projects_dir"`
}

// PickFolderRequest opens the native folder picker.
type PickFolderRequest struct {
	Title       string `json:"title,omitempty"`
	DefaultPath string `json:"default_path,omitempty"`
}

// PickFolderResult is the chosen folder; Cancelled when dismissed.
type PickFolderResult struct {
	Path      string `json:"path"`
	Cancelled bool   `json:"cancelled"`
}
