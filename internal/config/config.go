package config

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/spf13/viper"
)

const (
	HTTPReadHeaderTimeout = 10 * time.Second
	HTTPIdleTimeout       = 120 * time.Second
	HTTPMaxHeaderBytes    = 1 << 20
)

const readTimeoutBytesPerSecond int64 = 256 * 1024
const minReadTimeoutSeconds = 30

// Mode identifies which Nebi runtime is using the shared server stack.
type Mode string

const (
	ModeTeam  Mode = "team"
	ModeLocal Mode = "local"
)

// Config holds all application configuration
type Config struct {
	Mode       Mode             `mapstructure:"-"`
	Worker     WorkerConfig     `mapstructure:"worker"`
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Auth       AuthConfig       `mapstructure:"auth"`
	Log        LogConfig        `mapstructure:"log"`
	PixiPath   string           `mapstructure:"pixi_path"` // Custom pixi binary path (optional)
	Storage    StorageConfig    `mapstructure:"storage"`
	Limits     limits.Limits    `mapstructure:"limits"`
	Registries RegistriesConfig `mapstructure:"registries"`
}

// IsLocalMode returns true when the server is running in local/desktop mode.
func (c *Config) IsLocalMode() bool {
	return c.Mode == ModeLocal
}

// ServerConfig holds HTTP server configuration
// Host may be empty to allow "all interfaces" bind behavior.
type ServerConfig struct {
	Host               string `mapstructure:"host"` // Bind host/IP (e.g. "127.0.0.1", "0.0.0.0")
	Port               int    `mapstructure:"port"`
	Mode               string `mapstructure:"mode"`                 // "development" or "production"
	BasePath           string `mapstructure:"base_path"`            // URL path prefix (e.g. "/nebi")
	AllowedOrigins     string `mapstructure:"allowed_origins"`      // comma-separated non-loopback origins accepted in local mode and allowed as CSP frame-ancestors in all modes (e.g. "https://hub.example.com" when proxied by JupyterHub)
	ReadTimeoutSeconds int    `mapstructure:"read_timeout_seconds"` // Max seconds to read the full request, 0 disables
}

// AllowedOriginsList returns server.allowed_origins split on commas, with
// whitespace trimmed and empty entries dropped.
func (c *ServerConfig) AllowedOriginsList() []string {
	return splitCommaList(c.AllowedOrigins)
}

// ReadTimeout returns the configured HTTP request read timeout.
func (c *ServerConfig) ReadTimeout() time.Duration {
	if c.ReadTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(c.ReadTimeoutSeconds) * time.Second
}

// DefaultReadTimeoutSeconds derives a read timeout from the request body cap.
func DefaultReadTimeoutSeconds(requestBodyBytes int64) int {
	if requestBodyBytes <= 0 {
		return 0
	}
	seconds := int((requestBodyBytes + readTimeoutBytesPerSecond - 1) / readTimeoutBytesPerSecond)
	if seconds < minReadTimeoutSeconds {
		return minReadTimeoutSeconds
	}
	return seconds
}

// WorkerConfig holds background job concurrency configuration.
type WorkerConfig struct {
	MaxParallelJobs int `mapstructure:"max_parallel_jobs"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver          string `mapstructure:"driver"`            // "sqlite" or "postgres"
	DSN             string `mapstructure:"dsn"`               // Connection string
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`    // Maximum idle connections (Postgres)
	MaxOpenConns    int    `mapstructure:"max_open_conns"`    // Maximum open connections (Postgres)
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // Connection max lifetime in minutes (Postgres)
}

// Supported values for auth.type.
const (
	AuthTypeOIDC = "oidc"
	AuthTypeNone = "none"
)

// AuthConfig holds authentication configuration. In team mode nebi is an
// OIDC resource server: clients obtain access tokens from the identity
// provider and nebi only validates them.
type AuthConfig struct {
	Type             string `mapstructure:"type"`               // "oidc" (default) or "none" (no authentication, every request is an implicit admin)
	JWTSecret        string `mapstructure:"jwt_secret"`         // Secret the registry-credential encryption key is derived from
	OIDCIssuerURL    string `mapstructure:"oidc_issuer_url"`    // OIDC provider issuer URL; must match the "iss" claim of access tokens
	OIDCDiscoveryURL string `mapstructure:"oidc_discovery_url"` // Optional: URL for fetching .well-known/openid-configuration when it differs from the issuer (e.g. in-cluster Keycloak Service for back-channel calls); falls back to oidc_issuer_url when unset
	OIDCClientID     string `mapstructure:"oidc_client_id"`     // Public OIDC client used by the web UI, CLI and desktop app; access tokens must list it in "aud"
	OIDCScopes       string `mapstructure:"oidc_scopes"`        // Comma-separated scopes clients request (default: "openid,profile,email")
	OIDCAdminGroups  string `mapstructure:"oidc_admin_groups"`  // Comma-separated IdP groups whose members are nebi admins (default: "admin")
}

// OIDCScopesList returns auth.oidc_scopes split on commas, with whitespace
// trimmed and empty entries dropped.
func (c *AuthConfig) OIDCScopesList() []string {
	return splitCommaList(c.OIDCScopes)
}

// OIDCAdminGroupsList returns auth.oidc_admin_groups split on commas, with
// whitespace trimmed and empty entries dropped.
func (c *AuthConfig) OIDCAdminGroupsList() []string {
	return splitCommaList(c.OIDCAdminGroups)
}

func splitCommaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// LogConfig holds logging configuration
type LogConfig struct {
	Format string `mapstructure:"format"` // "json" or "text"
	Level  string `mapstructure:"level"`  // "debug", "info", "warn", "error"
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	ProjectsDir string `mapstructure:"projects_dir"` // Directory where projects are stored
}

// RegistriesConfig holds admin-provisioned OCI registry configuration.
// Entries are declarative: at boot they are reconciled into the database,
// marked config-managed, and locked against API/UI/CLI modification.
type RegistriesConfig struct {
	SeedDefault bool                  `mapstructure:"seed_default"` // seed the built-in quay.io/nebari_environments registry (default true)
	Entries     []RegistryEntryConfig `mapstructure:"entries"`
}

// RegistryEntryConfig is one admin-provisioned OCI registry. Only
// credentialless registry entries are supported; there are no credential fields.
type RegistryEntryConfig struct {
	Name       string `mapstructure:"name"`       // required, unique; reconciliation identity
	URL        string `mapstructure:"url"`        // required, e.g. "quay.io"
	Namespace  string `mapstructure:"namespace"`  // organization/namespace on the registry
	Default    bool   `mapstructure:"default"`    // at most one entry may set this
	Restricted bool   `mapstructure:"restricted"` // when true, only granted groups can use this registry
}

type loadOptions struct {
	mode Mode
}

// LoadOption customizes configuration loading.
type LoadOption func(*loadOptions)

// WithMode sets the explicit runtime mode for this process.
func WithMode(mode Mode) LoadOption {
	return func(opts *loadOptions) {
		opts.mode = mode
	}
}

// Load reads configuration from file and environment variables.
func Load(options ...LoadOption) (*Config, error) {
	opts := loadOptions{mode: ModeTeam}
	for _, option := range options {
		option(&opts)
	}
	if err := validateMode(opts.mode); err != nil {
		return nil, err
	}

	v := viper.New()

	// Set defaults for local development
	v.SetDefault("worker.max_parallel_jobs", max(1, runtime.NumCPU()/2))
	v.SetDefault("server.host", "")
	v.SetDefault("server.port", 8460)
	v.SetDefault("server.mode", "development")
	v.SetDefault("server.base_path", "")
	v.SetDefault("server.allowed_origins", "")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "./nebi.db")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", 60) // 60 minutes
	v.SetDefault("auth.type", AuthTypeOIDC)
	v.SetDefault("auth.jwt_secret", "change-me-in-production")
	v.SetDefault("auth.oidc_issuer_url", "")
	v.SetDefault("auth.oidc_discovery_url", "")
	v.SetDefault("auth.oidc_client_id", "")
	v.SetDefault("auth.oidc_scopes", "openid,profile,email")
	v.SetDefault("auth.oidc_admin_groups", "admin")
	v.SetDefault("log.format", "text")
	v.SetDefault("log.level", "info")
	v.SetDefault("storage.projects_dir", "./data/projects")
	defaultLimits := limits.Defaults()
	v.SetDefault("limits.request_body_bytes", defaultLimits.RequestBodyBytes)
	v.SetDefault("limits.manifest_bytes", defaultLimits.ManifestBytes)
	v.SetDefault("limits.lock_bytes", defaultLimits.LockBytes)
	v.SetDefault("limits.metadata_bytes", defaultLimits.MetadataBytes)
	v.SetDefault("limits.package_string_bytes", defaultLimits.PackageStringBytes)
	v.SetDefault("limits.active_jobs_per_user", defaultLimits.ActiveJobsPerUser)
	v.SetDefault("limits.active_jobs_per_project", defaultLimits.ActiveJobsPerProject)
	v.SetDefault("limits.active_jobs_global", defaultLimits.ActiveJobsGlobal)
	v.SetDefault("limits.job_timeout_seconds", defaultLimits.JobTimeoutSeconds)
	v.SetDefault("limits.job_cpu_seconds", defaultLimits.JobCPUSeconds)
	v.SetDefault("limits.job_storage_bytes", defaultLimits.JobStorageBytes)
	v.SetDefault("limits.job_log_bytes", defaultLimits.JobLogBytes)
	v.SetDefault("registries.seed_default", true)

	// Read from config file if exists
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/nebi/")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, using defaults
	}

	// Environment variables override
	v.SetEnvPrefix("NEBI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// NEBI_PACKAGE_MANAGER_PIXI_PATH is the pre-0.15 name, kept as an alias
	// so existing deployments keep working.
	_ = v.BindEnv("pixi_path", "NEBI_PIXI_PATH", "NEBI_PACKAGE_MANAGER_PIXI_PATH")

	// viper's AutomaticEnv + Unmarshal does not propagate env vars into
	// nested structs without explicit BindEnv. Bind each nested key so that
	// e.g. NEBI_STORAGE_PROJECTS_DIR overrides the projects_dir field.
	_ = v.BindEnv("worker.max_parallel_jobs", "NEBI_WORKER_MAX_PARALLEL_JOBS")
	_ = v.BindEnv("storage.projects_dir", "NEBI_STORAGE_PROJECTS_DIR")
	_ = v.BindEnv("server.host", "NEBI_SERVER_HOST")
	_ = v.BindEnv("server.port", "NEBI_SERVER_PORT")
	_ = v.BindEnv("server.mode", "NEBI_SERVER_MODE")
	_ = v.BindEnv("server.base_path", "NEBI_SERVER_BASE_PATH")
	_ = v.BindEnv("server.allowed_origins", "NEBI_SERVER_ALLOWED_ORIGINS")
	_ = v.BindEnv("server.read_timeout_seconds", "NEBI_SERVER_READ_TIMEOUT_SECONDS")
	_ = v.BindEnv("database.driver", "NEBI_DATABASE_DRIVER")
	_ = v.BindEnv("database.dsn", "NEBI_DATABASE_DSN")
	_ = v.BindEnv("auth.type", "NEBI_AUTH_TYPE")
	_ = v.BindEnv("auth.jwt_secret", "NEBI_AUTH_JWT_SECRET")
	_ = v.BindEnv("auth.oidc_issuer_url", "NEBI_AUTH_OIDC_ISSUER_URL")
	_ = v.BindEnv("auth.oidc_discovery_url", "NEBI_AUTH_OIDC_DISCOVERY_URL")
	_ = v.BindEnv("auth.oidc_client_id", "NEBI_AUTH_OIDC_CLIENT_ID")
	_ = v.BindEnv("auth.oidc_scopes", "NEBI_AUTH_OIDC_SCOPES")
	_ = v.BindEnv("auth.oidc_admin_groups", "NEBI_AUTH_OIDC_ADMIN_GROUPS")
	_ = v.BindEnv("log.format", "NEBI_LOG_FORMAT")
	_ = v.BindEnv("log.level", "NEBI_LOG_LEVEL")
	_ = v.BindEnv("limits.request_body_bytes", "NEBI_LIMITS_REQUEST_BODY_BYTES")
	_ = v.BindEnv("limits.manifest_bytes", "NEBI_LIMITS_MANIFEST_BYTES")
	_ = v.BindEnv("limits.lock_bytes", "NEBI_LIMITS_LOCK_BYTES")
	_ = v.BindEnv("limits.metadata_bytes", "NEBI_LIMITS_METADATA_BYTES")
	_ = v.BindEnv("limits.package_string_bytes", "NEBI_LIMITS_PACKAGE_STRING_BYTES")
	_ = v.BindEnv("limits.active_jobs_per_user", "NEBI_LIMITS_ACTIVE_JOBS_PER_USER")
	_ = v.BindEnv("limits.active_jobs_per_project", "NEBI_LIMITS_ACTIVE_JOBS_PER_PROJECT")
	_ = v.BindEnv("limits.active_jobs_global", "NEBI_LIMITS_ACTIVE_JOBS_GLOBAL")
	_ = v.BindEnv("limits.job_timeout_seconds", "NEBI_LIMITS_JOB_TIMEOUT_SECONDS")
	_ = v.BindEnv("limits.job_cpu_seconds", "NEBI_LIMITS_JOB_CPU_SECONDS")
	_ = v.BindEnv("limits.job_storage_bytes", "NEBI_LIMITS_JOB_STORAGE_BYTES")
	_ = v.BindEnv("limits.job_log_bytes", "NEBI_LIMITS_JOB_LOG_BYTES")

	readTimeoutExplicit := v.IsSet("server.read_timeout_seconds")
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	cfg.Mode = opts.mode
	if cfg.Worker.MaxParallelJobs < 1 {
		return nil, fmt.Errorf("worker.max_parallel_jobs must be at least 1")
	}
	if err := cfg.Limits.Validate(); err != nil {
		return nil, err
	}
	if !readTimeoutExplicit {
		cfg.Server.ReadTimeoutSeconds = DefaultReadTimeoutSeconds(cfg.Limits.RequestBodyBytes)
	}
	if cfg.Server.ReadTimeoutSeconds < 0 {
		return nil, fmt.Errorf("server.read_timeout_seconds must be non-negative")
	}

	// Normalize base path: ensure leading slash, strip trailing slash
	if cfg.Server.BasePath != "" {
		cfg.Server.BasePath = "/" + strings.Trim(cfg.Server.BasePath, "/")
	}

	if err := normalizeRegistries(&cfg.Registries); err != nil {
		return nil, err
	}

	// Team mode stores registry credentials encrypted with a key derived from
	// this secret, so it must not be empty, the shipped default, or too short
	// to resist brute force. Local mode is exempt.
	if !cfg.IsLocalMode() {
		if err := validateTeamModeJWTSecret(cfg.Auth.JWTSecret); err != nil {
			return nil, err
		}
		if err := validateTeamModeAuth(&cfg.Auth); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

func validateMode(mode Mode) error {
	switch mode {
	case ModeTeam, ModeLocal:
		return nil
	default:
		return fmt.Errorf("invalid mode %q: must be %q or %q", mode, ModeLocal, ModeTeam)
	}
}

const (
	defaultJWTSecret   = "change-me-in-production"
	minJWTSecretLength = 32
)

func validateTeamModeJWTSecret(secret string) error {
	if secret == "" {
		return fmt.Errorf("auth.jwt_secret (NEBI_AUTH_JWT_SECRET) must be set in team mode")
	}
	if secret == defaultJWTSecret {
		return fmt.Errorf("auth.jwt_secret (NEBI_AUTH_JWT_SECRET) must not be the default value %q in team mode", defaultJWTSecret)
	}
	if len(secret) < minJWTSecretLength {
		return fmt.Errorf("auth.jwt_secret (NEBI_AUTH_JWT_SECRET) must be at least %d characters in team mode", minJWTSecretLength)
	}
	return nil
}

func validateTeamModeAuth(ac *AuthConfig) error {
	ac.Type = strings.ToLower(strings.TrimSpace(ac.Type))
	switch ac.Type {
	case AuthTypeNone:
		return nil
	case AuthTypeOIDC:
		if strings.TrimSpace(ac.OIDCIssuerURL) == "" {
			return fmt.Errorf("auth.oidc_issuer_url (NEBI_AUTH_OIDC_ISSUER_URL) must be set when auth.type is %q", AuthTypeOIDC)
		}
		if strings.TrimSpace(ac.OIDCClientID) == "" {
			return fmt.Errorf("auth.oidc_client_id (NEBI_AUTH_OIDC_CLIENT_ID) must be set when auth.type is %q", AuthTypeOIDC)
		}
		return nil
	default:
		return fmt.Errorf("invalid auth.type %q: must be %q or %q", ac.Type, AuthTypeOIDC, AuthTypeNone)
	}
}

// normalizeRegistries validates the registries section.
func normalizeRegistries(rc *RegistriesConfig) error {
	seen := make(map[string]bool, len(rc.Entries))
	defaults := 0
	for i := range rc.Entries {
		e := &rc.Entries[i]
		e.Name = strings.TrimSpace(e.Name)
		e.URL = strings.TrimSpace(e.URL)
		if e.Name == "" {
			return fmt.Errorf("registries.entries[%d]: name is required", i)
		}
		if seen[e.Name] {
			return fmt.Errorf("registries.entries: duplicate name %q", e.Name)
		}
		seen[e.Name] = true
		if e.URL == "" {
			return fmt.Errorf("registries.entries[%d] (%s): url is required", i, e.Name)
		}
		if e.Default {
			defaults++
		}
	}
	if defaults > 1 {
		return fmt.Errorf("registries.entries: at most one entry may set default: true")
	}
	return nil
}
