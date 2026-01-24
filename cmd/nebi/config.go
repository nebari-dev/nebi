package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/cliclient"
	"gopkg.in/yaml.v3"
)

// CLIConfig holds the CLI configuration.
type CLIConfig struct {
	ServerURL string `yaml:"server_url,omitempty"`
	Token     string `yaml:"token,omitempty"`
}

var (
	configDir    string
	cachedConfig *CLIConfig
	apiClient    *cliclient.Client
)

// getConfigDir returns the platform-specific config directory.
func getConfigDir() (string, error) {
	if configDir != "" {
		return configDir, nil
	}

	if envDir := os.Getenv("NEBI_CONFIG_DIR"); envDir != "" {
		configDir = envDir
		return configDir, nil
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}

	configDir = filepath.Join(baseDir, "nebi")
	return configDir, nil
}

// getConfigPath returns the path to the config file.
func getConfigPath() (string, error) {
	dir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// loadConfig loads the CLI config from disk.
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
			cachedConfig = &CLIConfig{}
			applyEnvOverrides(cachedConfig)
			return cachedConfig, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cachedConfig = &cfg
	applyEnvOverrides(cachedConfig)
	return cachedConfig, nil
}

// applyEnvOverrides overrides config values with environment variables if set.
func applyEnvOverrides(cfg *CLIConfig) {
	if v := os.Getenv("NEBI_SERVER_URL"); v != "" {
		cfg.ServerURL = v
	}
	if v := os.Getenv("NEBI_TOKEN"); v != "" {
		cfg.Token = v
	}
}

// saveConfig saves the CLI config to disk.
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

// getAPIClient returns a configured API client.
func getAPIClient() (*cliclient.Client, error) {
	if apiClient != nil {
		return apiClient, nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("not logged in. Run 'nebi login <url>' first")
	}

	apiClient = cliclient.New(cfg.ServerURL, cfg.Token)
	return apiClient, nil
}

// getAuthContext returns a context for authenticated requests.
func getAuthContext() (context.Context, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("not logged in. Run 'nebi login <url>' first")
	}

	return context.Background(), nil
}

// mustGetClient returns the API client or exits with error.
func mustGetClient() *cliclient.Client {
	c, err := getAPIClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}
	return c
}

// mustGetAuthContext returns auth context or exits with error.
func mustGetAuthContext() context.Context {
	ctx, err := getAuthContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}
	return ctx
}
