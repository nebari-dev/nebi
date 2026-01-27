// Package localserver manages the lifecycle of a local nebi server instance.
// It handles reading/writing server state, spawning the server process,
// lock file management, port selection, and token generation.
package localserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ServerState represents the runtime state of a local server instance.
type ServerState struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"started_at"`
}

// DefaultBasePort is the first port to try when starting the server.
const DefaultBasePort = 8460

// MaxPortAttempts is the maximum number of ports to try before giving up.
const MaxPortAttempts = 10

// getDataDir returns the platform-specific data directory for nebi.
// Always uses ~/.local/share/nebi based on $HOME to avoid issues with
// sandboxed environments (e.g., snap-confined VS Code setting XDG_DATA_HOME
// to a snap-specific path).
func getDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(home, ".local", "share", "nebi"), nil
}

// GetStatePath returns the path to the server state file.
func GetStatePath() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "server.state"), nil
}

// GetLockPath returns the path to the spawn lock file.
func GetLockPath() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "spawn.lock"), nil
}

// GetDBPath returns the path to the local SQLite database.
func GetDBPath() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nebi.db"), nil
}

// ReadState reads the server state from disk.
// Returns nil, nil if the state file does not exist.
func ReadState() (*ServerState, error) {
	statePath, err := GetStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read server state: %w", err)
	}

	var state ServerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse server state: %w", err)
	}

	return &state, nil
}

// WriteState writes the server state to disk.
func WriteState(state *ServerState) error {
	statePath, err := GetStatePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal server state: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write server state: %w", err)
	}

	return nil
}

// RemoveState removes the server state file.
func RemoveState() error {
	statePath, err := GetStatePath()
	if err != nil {
		return err
	}

	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove server state: %w", err)
	}

	return nil
}
