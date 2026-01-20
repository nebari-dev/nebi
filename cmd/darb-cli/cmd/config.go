package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aktech/darb/cli/client"
	"gopkg.in/yaml.v3"
)

// ServerConfig holds configuration for a single server
type ServerConfig struct {
	URL   string `yaml:"url,omitempty"`   // Empty for local server
	Token string `yaml:"token,omitempty"` // JWT token
}

// CLIConfig holds the CLI configuration with multi-server support
type CLIConfig struct {
	CurrentServer string                  `yaml:"current_server"` // "local" or server name
	Servers       map[string]ServerConfig `yaml:"servers"`        // Named servers
}

// legacyConfig represents the old config format for migration
type legacyConfig struct {
	ServerURL string `yaml:"server_url,omitempty"`
	Token     string `yaml:"token,omitempty"`
}

var (
	configDir    string
	dataDir      string
	cachedConfig *CLIConfig
	apiClient    *client.APIClient
	cachedToken  string // Cached token for auth context
)

// getConfigDir returns the platform-specific config directory (~/.config/nebi)
func getConfigDir() (string, error) {
	if configDir != "" {
		return configDir, nil
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}

	configDir = filepath.Join(baseDir, "nebi")
	return configDir, nil
}

// getDataDir returns the platform-specific data directory (~/.local/share/nebi)
func getDataDir() (string, error) {
	if dataDir != "" {
		return dataDir, nil
	}

	// XDG_DATA_HOME or default to ~/.local/share
	baseDir := os.Getenv("XDG_DATA_HOME")
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".local", "share")
	}

	dataDir = filepath.Join(baseDir, "nebi")
	return dataDir, nil
}

// getConfigPath returns the path to the config file
func getConfigPath() (string, error) {
	dir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// loadConfig loads the CLI config from disk, migrating from old format if needed
func loadConfig() (*CLIConfig, error) {
	if cachedConfig != nil {
		return cachedConfig, nil
	}

	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config with local server
			cachedConfig = &CLIConfig{
				CurrentServer: "local",
				Servers:       make(map[string]ServerConfig),
			}
			return cachedConfig, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Try parsing as new format first
	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Check if this is old format (has no servers map but might have server_url at root level)
	// We detect old format by checking if Servers is nil/empty and trying legacy parse
	if cfg.Servers == nil || len(cfg.Servers) == 0 {
		var legacy legacyConfig
		if err := yaml.Unmarshal(data, &legacy); err == nil && legacy.ServerURL != "" {
			// Migrate from old format
			cfg = CLIConfig{
				CurrentServer: "default",
				Servers: map[string]ServerConfig{
					"default": {
						URL:   legacy.ServerURL,
						Token: legacy.Token,
					},
				},
			}
			// Save migrated config
			if saveErr := saveConfig(&cfg); saveErr != nil {
				// Log but don't fail - we can still use the migrated config in memory
				fmt.Fprintf(os.Stderr, "Warning: failed to save migrated config: %v\n", saveErr)
			}
		}
	}

	// Ensure Servers map is initialized
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]ServerConfig)
	}

	// Default to local if not set
	if cfg.CurrentServer == "" {
		cfg.CurrentServer = "local"
	}

	cachedConfig = &cfg
	return cachedConfig, nil
}

// saveConfig saves the CLI config to disk
func saveConfig(cfg *CLIConfig) error {
	dir, err := getConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	cachedConfig = cfg
	return nil
}

// getAPIClient returns a configured API client
func getAPIClient() (*client.APIClient, error) {
	if apiClient != nil {
		return apiClient, nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	var serverURL string

	if cfg.CurrentServer == "local" {
		// Auto-spawn local server and get connection info
		url, token, err := ensureLocalServer()
		if err != nil {
			return nil, fmt.Errorf("failed to start local server: %w", err)
		}
		serverURL = url
		cachedToken = token
	} else {
		// Use configured remote server
		serverCfg, ok := cfg.Servers[cfg.CurrentServer]
		if !ok {
			return nil, fmt.Errorf("server %q not found in config", cfg.CurrentServer)
		}
		if serverCfg.URL == "" {
			return nil, fmt.Errorf("server %q has no URL configured", cfg.CurrentServer)
		}
		if serverCfg.Token == "" {
			return nil, fmt.Errorf("not logged in to %q. Run 'nebi server login %s' first",
				cfg.CurrentServer, serverCfg.URL)
		}
		serverURL = serverCfg.URL
		cachedToken = serverCfg.Token
	}

	clientCfg := client.NewConfiguration()
	clientCfg.Servers = client.ServerConfigurations{
		{URL: serverURL + "/api/v1"},
	}

	apiClient = client.NewAPIClient(clientCfg)
	return apiClient, nil
}

// getAuthContext returns a context with authentication token
func getAuthContext() (context.Context, error) {
	// Ensure API client is initialized (this also sets cachedToken)
	if _, err := getAPIClient(); err != nil {
		return nil, err
	}

	if cachedToken == "" {
		return nil, fmt.Errorf("no authentication token available")
	}

	ctx := context.WithValue(context.Background(), client.ContextAPIKeys, map[string]client.APIKey{
		"BearerAuth": {
			Key:    cachedToken,
			Prefix: "Bearer",
		},
	})

	return ctx, nil
}

// mustGetClient returns the API client or exits with error
func mustGetClient() *client.APIClient {
	c, err := getAPIClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return c
}

// mustGetAuthContext returns auth context or exits with error
func mustGetAuthContext() context.Context {
	ctx, err := getAuthContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return ctx
}
