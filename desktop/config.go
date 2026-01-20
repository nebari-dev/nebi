package desktop

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"

	"github.com/openteams-ai/darb/internal/config"
)

// GetDataDir returns the platform-specific application data directory
// - macOS: ~/Library/Application Support/Darb/
// - Windows: %APPDATA%\Darb\
// - Linux: ~/.local/share/darb/
func GetDataDir() (string, error) {
	var dataDir string

	switch runtime.GOOS {
	case "darwin":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(homeDir, "Library", "Application Support", "Darb")

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		dataDir = filepath.Join(appData, "Darb")

	default: // Linux and other Unix-like systems
		// Check XDG_DATA_HOME first
		xdgDataHome := os.Getenv("XDG_DATA_HOME")
		if xdgDataHome != "" {
			dataDir = filepath.Join(xdgDataHome, "darb")
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			dataDir = filepath.Join(homeDir, ".local", "share", "darb")
		}
	}

	// Ensure the directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}

	return dataDir, nil
}

// GetEnvironmentsDir returns the directory where environments are stored
func GetEnvironmentsDir() (string, error) {
	dataDir, err := GetDataDir()
	if err != nil {
		return "", err
	}

	envDir := filepath.Join(dataDir, "environments")
	if err := os.MkdirAll(envDir, 0755); err != nil {
		return "", err
	}

	return envDir, nil
}

// GetLogDir returns the directory where logs are stored
func GetLogDir() (string, error) {
	dataDir, err := GetDataDir()
	if err != nil {
		return "", err
	}

	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", err
	}

	return logDir, nil
}

// GetDatabasePath returns the path to the SQLite database file
func GetDatabasePath() (string, error) {
	dataDir, err := GetDataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dataDir, "darb.db"), nil
}

// NewDesktopConfig creates a configuration suitable for the desktop app
// Uses SQLite database, memory queue, and platform-specific paths
func NewDesktopConfig() (*config.Config, error) {
	dbPath, err := GetDatabasePath()
	if err != nil {
		return nil, err
	}

	envDir, err := GetEnvironmentsDir()
	if err != nil {
		return nil, err
	}

	return &config.Config{
		Server: config.ServerConfig{
			Port: 8460,
			Mode: "production",
		},
		Database: config.DatabaseConfig{
			Driver:          "sqlite",
			DSN:             dbPath,
			MaxIdleConns:    5,
			MaxOpenConns:    30,
			ConnMaxLifetime: 60,
		},
		Auth: config.AuthConfig{
			Type:      "basic",
			JWTSecret: generateJWTSecret(),
		},
		Queue: config.QueueConfig{
			Type: "memory", // Desktop app uses memory queue (single user)
		},
		Log: config.LogConfig{
			Format: "text",
			Level:  "info",
		},
		PackageManager: config.PackageManagerConfig{
			DefaultType: "pixi",
		},
		Storage: config.StorageConfig{
			EnvironmentsDir: envDir,
		},
	}, nil
}

// generateJWTSecret generates a deterministic but unique JWT secret for the desktop app
// This is stored in the data directory so it persists across restarts
func generateJWTSecret() string {
	dataDir, err := GetDataDir()
	if err != nil {
		// Fallback to a static secret if we can't get the data directory
		return "darb-desktop-fallback-secret"
	}

	secretFile := filepath.Join(dataDir, ".jwt_secret")

	// Try to read existing secret
	if data, err := os.ReadFile(secretFile); err == nil && len(data) > 0 {
		return string(data)
	}

	// Generate a new secret
	secret := generateRandomSecret(32)

	// Save it for future use
	_ = os.WriteFile(secretFile, []byte(secret), 0600)

	return secret
}

// generateRandomSecret generates a random alphanumeric secret of the given length
func generateRandomSecret(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	// Use crypto/rand for secure random generation
	// Fall back to a timestamp-based seed if that fails
	if _, err := cryptoRandRead(b); err != nil {
		// Simple fallback using the hostname and current time
		hostname, _ := os.Hostname()
		seed := hostname + string(rune(os.Getpid()))
		for i := range b {
			b[i] = charset[(int(seed[i%len(seed)])+i)%len(charset)]
		}
		return string(b)
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

// cryptoRandRead uses crypto/rand for secure random generation
func cryptoRandRead(b []byte) (int, error) {
	return rand.Read(b)
}
