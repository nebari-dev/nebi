package localstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Store manages the local nebi index and snapshots.
type Store struct {
	dataDir string
}

// NewStore creates a Store using the default platform data directory.
func NewStore() (*Store, error) {
	dataDir, err := defaultDataDir()
	if err != nil {
		return nil, fmt.Errorf("determining data directory: %w", err)
	}
	return &Store{dataDir: dataDir}, nil
}

// NewStoreWithDir creates a Store with a custom data directory (for testing).
func NewStoreWithDir(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// DataDir returns the store's data directory.
func (s *Store) DataDir() string {
	return s.dataDir
}

// IndexPath returns the path to index.json.
func (s *Store) IndexPath() string {
	return filepath.Join(s.dataDir, "index.json")
}

// LoadIndex reads the index from disk. Returns an empty index if the file doesn't exist.
func (s *Store) LoadIndex() (*Index, error) {
	data, err := os.ReadFile(s.IndexPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Index{Workspaces: make(map[string]*Workspace)}, nil
		}
		return nil, fmt.Errorf("reading index: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}
	if idx.Workspaces == nil {
		idx.Workspaces = make(map[string]*Workspace)
	}
	return &idx, nil
}

// SaveIndex writes the index to disk, creating directories as needed.
func (s *Store) SaveIndex(idx *Index) error {
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling index: %w", err)
	}

	if err := os.WriteFile(s.IndexPath(), data, 0644); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}
	return nil
}

// SnapshotDir returns the snapshot directory for a given workspace ID.
func (s *Store) SnapshotDir(workspaceID string) string {
	return filepath.Join(s.dataDir, "snapshots", workspaceID)
}

// defaultDataDir returns ~/.local/share/nebi/ on Linux, platform equivalent elsewhere.
func defaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "nebi"), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, "nebi"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "nebi"), nil
	default: // linux and others
		return filepath.Join(home, ".local", "share", "nebi"), nil
	}
}
