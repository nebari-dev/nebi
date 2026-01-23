package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/aktech/darb/internal/localserver"
	"gopkg.in/yaml.v3"
)

// CLIConfig holds the CLI configuration.
type CLIConfig struct {
	ServerURL string `yaml:"server_url,omitempty"`
	Token     string `yaml:"token,omitempty"`
	UseLocal  bool   `yaml:"use_local,omitempty"` // User confirmed local server mode
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
			return cachedConfig, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cachedConfig = &cfg
	return cachedConfig, nil
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
// If no explicit server URL is configured, it prompts the user and then ensures
// a local server is running, connecting to it using the local token.
func getAPIClient() (*cliclient.Client, error) {
	if apiClient != nil {
		return apiClient, nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	// If an explicit server URL is configured, use it (remote mode).
	if cfg.ServerURL != "" {
		apiClient = cliclient.New(cfg.ServerURL, cfg.Token)
		return apiClient, nil
	}

	// No server configured. If user hasn't previously confirmed local mode, prompt them.
	if !cfg.UseLocal {
		confirmed, err := promptLocalServer()
		if err != nil {
			return nil, err
		}
		if !confirmed {
			return nil, fmt.Errorf("no remote server configured. Run 'nebi login <url>' to connect to a remote server")
		}
		// Save the user's choice so we don't prompt again.
		cfg.UseLocal = true
		if err := saveConfig(cfg); err != nil {
			return nil, fmt.Errorf("failed to save config: %w", err)
		}
	}

	// Use local server mode.
	info, err := localserver.EnsureRunning(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to start local server: %w", err)
	}

	fmt.Printf("Local server started on port %d (will auto-shutdown after 15 minutes of inactivity)\n", info.Port)

	apiClient = cliclient.New(info.URL(), info.Token)
	return apiClient, nil
}

// promptLocalServer asks the user if they want to use nebi in local mode.
func promptLocalServer() (bool, error) {
	fmt.Println("No remote server configured. Would you like to use nebi in local mode?")
	fmt.Println("  (To connect to a remote server, run 'nebi login <url>')")
	fmt.Print("\nUse local mode? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes", nil
}

// getAuthContext returns a context for authenticated requests.
// In local mode, this always succeeds since the token is managed internally.
func getAuthContext() (context.Context, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	// If using a remote server, require a stored token.
	if cfg.ServerURL != "" && cfg.Token == "" {
		return nil, fmt.Errorf("not logged in. Run 'nebi login <url>' first")
	}

	return context.Background(), nil
}

// mustGetClient returns the API client or exits with error.
func mustGetClient() *cliclient.Client {
	c, err := getAPIClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return c
}

// mustGetAuthContext returns auth context or exits with error.
func mustGetAuthContext() context.Context {
	ctx, err := getAuthContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return ctx
}
