package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Database       DatabaseConfig       `mapstructure:"database"`
	Auth           AuthConfig           `mapstructure:"auth"`
	Queue          QueueConfig          `mapstructure:"queue"`
	Log            LogConfig            `mapstructure:"log"`
	PackageManager PackageManagerConfig `mapstructure:"package_manager"`
	Storage        StorageConfig        `mapstructure:"storage"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // "development" or "production"
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver          string `mapstructure:"driver"`            // "sqlite" or "postgres"
	DSN             string `mapstructure:"dsn"`               // Connection string
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`    // Maximum idle connections (Postgres)
	MaxOpenConns    int    `mapstructure:"max_open_conns"`    // Maximum open connections (Postgres)
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // Connection max lifetime in minutes (Postgres)
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Type      string `mapstructure:"type"`       // "basic" or "oidc"
	JWTSecret string `mapstructure:"jwt_secret"` // Secret for JWT signing
}

// QueueConfig holds job queue configuration
type QueueConfig struct {
	Type       string `mapstructure:"type"`        // "memory" or "valkey"
	ValkeyAddr string `mapstructure:"valkey_addr"` // Valkey address (if type=valkey), e.g., "localhost:6379"
}

// LogConfig holds logging configuration
type LogConfig struct {
	Format string `mapstructure:"format"` // "json" or "text"
	Level  string `mapstructure:"level"`  // "debug", "info", "warn", "error"
}

// PackageManagerConfig holds package manager configuration
type PackageManagerConfig struct {
	DefaultType string `mapstructure:"default_type"` // "pixi" or "uv"
	PixiPath    string `mapstructure:"pixi_path"`    // Custom pixi binary path (optional)
	UvPath      string `mapstructure:"uv_path"`      // Custom uv binary path (optional)
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	EnvironmentsDir string `mapstructure:"environments_dir"` // Directory where environments are stored
}

// Load reads configuration from file and environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults for local development
	v.SetDefault("server.port", 8460)
	v.SetDefault("server.mode", "development")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "./darb.db")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", 60) // 60 minutes
	v.SetDefault("auth.type", "basic")
	v.SetDefault("auth.jwt_secret", "change-me-in-production")
	v.SetDefault("queue.type", "memory")
	v.SetDefault("queue.valkey_addr", "localhost:6379")
	v.SetDefault("log.format", "text")
	v.SetDefault("log.level", "info")
	v.SetDefault("package_manager.default_type", "pixi")
	v.SetDefault("storage.environments_dir", "./data/environments")

	// Read from config file if exists
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/darb/")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, using defaults
	}

	// Environment variables override
	v.SetEnvPrefix("DARB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}
