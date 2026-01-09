package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile   string
	serverURL string
	apiClient *client.APIClient
	configDir string // cached config directory path
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "darb",
	Short: "Darb CLI - Multi-user environment management",
	Long: `Darb CLI is a command-line interface for managing Pixi environments.

It allows you to create, manage, and publish environments to OCI registries.

Examples:
  darb login
  darb environments list
  darb environments create --name myenv
  darb environments publish --id <env-id> --registry <registry-id>`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is platform config dir)")
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "Darb server URL")

	viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
}

// getConfigDir returns the platform-specific config directory for darb
func getConfigDir() (string, error) {
	if configDir != "" {
		return configDir, nil
	}

	// Use platform-specific config directory
	// Linux: ~/.config/darb (or $XDG_CONFIG_HOME/darb)
	// macOS: ~/Library/Application Support/darb
	// Windows: %AppData%/darb
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}

	configDir = filepath.Join(baseDir, "darb")
	return configDir, nil
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		dir, err := getConfigDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error getting config directory:", err)
			os.Exit(1)
		}

		viper.AddConfigPath(dir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	// Environment variables
	viper.SetEnvPrefix("DARB")
	viper.AutomaticEnv()

	// Read config file if exists
	if err := viper.ReadInConfig(); err == nil {
		// Config file found and loaded
	}
}

// ensureServerConfigured prompts for server URL if not configured
func ensureServerConfigured() error {
	server := viper.GetString("server")
	if server != "" {
		return nil
	}

	fmt.Println("No Darb server configured.")
	fmt.Print("Enter server URL (e.g., https://darb.example.com): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read server URL: %w", err)
	}

	server = strings.TrimSpace(input)
	if server == "" {
		return fmt.Errorf("server URL is required")
	}

	// Ensure URL has scheme
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "https://" + server
	}

	viper.Set("server", server)
	if err := saveConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save config: %v\n", err)
	} else {
		dir, _ := getConfigDir()
		fmt.Printf("Server URL saved to %s/config.yaml\n\n", dir)
	}

	return nil
}

// getAPIClient returns a configured API client
func getAPIClient() *client.APIClient {
	if apiClient != nil {
		return apiClient
	}

	// Ensure server is configured
	if err := ensureServerConfigured(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cfg := client.NewConfiguration()
	cfg.Servers = client.ServerConfigurations{
		{
			URL: viper.GetString("server") + "/api/v1",
		},
	}

	apiClient = client.NewAPIClient(cfg)
	return apiClient
}

// getAuthContext returns a context with authentication
func getAuthContext() context.Context {
	token := viper.GetString("token")
	if token == "" {
		return context.Background()
	}

	return context.WithValue(context.Background(), client.ContextAPIKeys, map[string]client.APIKey{
		"BearerAuth": {
			Key:    token,
			Prefix: "Bearer",
		},
	})
}

// saveConfig saves the current configuration to file
func saveConfig() error {
	dir, err := getConfigDir()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	return viper.WriteConfigAs(configPath)
}
