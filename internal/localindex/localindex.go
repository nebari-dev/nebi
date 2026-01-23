// Package localindex provides CRUD operations for the local workspace index.
//
// The local index is stored at ~/.local/share/nebi/index.json and tracks all
// workspaces that have been pulled to the local machine, including their
// origin information, layer digests for drift detection, and optional aliases.
package localindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// CurrentVersion is the current schema version of the index file.
	CurrentVersion = 1

	// DefaultIndexDir is the default directory for the index file.
	DefaultIndexDir = ".local/share/nebi"

	// IndexFileName is the name of the index file.
	IndexFileName = "index.json"
)

// Index represents the local workspace index.
type Index struct {
	Version    int              `json:"version"`
	Workspaces []WorkspaceEntry `json:"workspaces"`
	Aliases    map[string]Alias `json:"aliases,omitempty"`
}

// WorkspaceEntry represents a single workspace entry in the index.
type WorkspaceEntry struct {
	Workspace       string            `json:"workspace"`
	Tag             string            `json:"tag"`
	RegistryURL     string            `json:"registry_url,omitempty"`
	ServerURL       string            `json:"server_url"`
	ServerVersionID int32             `json:"server_version_id"`
	Path            string            `json:"path"`
	IsGlobal        bool              `json:"is_global"`
	PulledAt        time.Time         `json:"pulled_at"`
	ManifestDigest  string            `json:"manifest_digest,omitempty"`
	Layers          map[string]string `json:"layers,omitempty"`
}

// Alias maps a user-friendly name to a UUID + tag in global storage.
type Alias struct {
	UUID string `json:"uuid"`
	Tag  string `json:"tag"`
}

// Store provides CRUD operations for the local index.
type Store struct {
	mu       sync.Mutex
	indexDir string
}

// NewStore creates a new Store with the default index directory.
func NewStore() *Store {
	homeDir, _ := os.UserHomeDir()
	return &Store{
		indexDir: filepath.Join(homeDir, DefaultIndexDir),
	}
}

// NewStoreWithDir creates a new Store with a custom index directory.
// This is primarily useful for testing.
func NewStoreWithDir(dir string) *Store {
	return &Store{
		indexDir: dir,
	}
}

// IndexPath returns the full path to the index file.
func (s *Store) IndexPath() string {
	return filepath.Join(s.indexDir, IndexFileName)
}

// Load reads the index from disk. Returns an empty index if the file doesn't exist.
func (s *Store) Load() (*Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadUnsafe()
}

// loadUnsafe reads the index without holding the lock (caller must hold lock).
func (s *Store) loadUnsafe() (*Index, error) {
	path := s.IndexPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{
				Version:    CurrentVersion,
				Workspaces: []WorkspaceEntry{},
				Aliases:    make(map[string]Alias),
			}, nil
		}
		return nil, fmt.Errorf("failed to read index file: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse index file: %w", err)
	}

	if idx.Aliases == nil {
		idx.Aliases = make(map[string]Alias)
	}

	return &idx, nil
}

// Save writes the index to disk, creating the directory if needed.
func (s *Store) Save(idx *Index) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnsafe(idx)
}

// saveUnsafe writes the index without holding the lock (caller must hold lock).
func (s *Store) saveUnsafe(idx *Index) error {
	if err := os.MkdirAll(s.indexDir, 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	path := s.IndexPath()
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	return nil
}

// AddEntry adds or updates a workspace entry in the index.
// If an entry with the same path already exists, it is replaced.
func (s *Store) AddEntry(entry WorkspaceEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}

	// Replace existing entry with same path
	found := false
	for i, existing := range idx.Workspaces {
		if existing.Path == entry.Path {
			idx.Workspaces[i] = entry
			found = true
			break
		}
	}
	if !found {
		idx.Workspaces = append(idx.Workspaces, entry)
	}

	return s.saveUnsafe(idx)
}

// RemoveByPath removes the entry at the given path.
// Returns true if an entry was removed, false if not found.
func (s *Store) RemoveByPath(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return false, err
	}

	found := false
	filtered := make([]WorkspaceEntry, 0, len(idx.Workspaces))
	for _, entry := range idx.Workspaces {
		if entry.Path == path {
			found = true
			continue
		}
		filtered = append(filtered, entry)
	}

	if !found {
		return false, nil
	}

	idx.Workspaces = filtered
	return true, s.saveUnsafe(idx)
}

// FindByPath returns the entry at the given path, or nil if not found.
func (s *Store) FindByPath(path string) (*WorkspaceEntry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	for i := range idx.Workspaces {
		if idx.Workspaces[i].Path == path {
			return &idx.Workspaces[i], nil
		}
	}
	return nil, nil
}

// FindByWorkspaceTag returns all entries matching a workspace name and tag.
func (s *Store) FindByWorkspaceTag(workspace, tag string) ([]WorkspaceEntry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	var matches []WorkspaceEntry
	for _, entry := range idx.Workspaces {
		if entry.Workspace == workspace && entry.Tag == tag {
			matches = append(matches, entry)
		}
	}
	return matches, nil
}

// FindGlobal returns all entries matching a workspace name and tag that are global.
func (s *Store) FindGlobal(workspace, tag string) (*WorkspaceEntry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	for i := range idx.Workspaces {
		if idx.Workspaces[i].Workspace == workspace &&
			idx.Workspaces[i].Tag == tag &&
			idx.Workspaces[i].IsGlobal {
			return &idx.Workspaces[i], nil
		}
	}
	return nil, nil
}

// ListAll returns all workspace entries.
func (s *Store) ListAll() ([]WorkspaceEntry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}
	return idx.Workspaces, nil
}

// SetAlias sets a user-friendly alias for a global workspace.
func (s *Store) SetAlias(name string, alias Alias) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}

	idx.Aliases[name] = alias
	return s.saveUnsafe(idx)
}

// RemoveAlias removes an alias by name.
// Returns true if the alias was removed, false if not found.
func (s *Store) RemoveAlias(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return false, err
	}

	if _, exists := idx.Aliases[name]; !exists {
		return false, nil
	}

	delete(idx.Aliases, name)
	return true, s.saveUnsafe(idx)
}

// GetAlias returns the alias for the given name, or nil if not found.
func (s *Store) GetAlias(name string) (*Alias, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	alias, exists := idx.Aliases[name]
	if !exists {
		return nil, nil
	}
	return &alias, nil
}

// ListAliases returns all aliases.
func (s *Store) ListAliases() (map[string]Alias, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}
	return idx.Aliases, nil
}

// Prune removes entries whose paths no longer exist on disk.
// Returns the list of removed entries.
func (s *Store) Prune() ([]WorkspaceEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}

	var pruned []WorkspaceEntry
	filtered := make([]WorkspaceEntry, 0, len(idx.Workspaces))
	for _, entry := range idx.Workspaces {
		if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
			pruned = append(pruned, entry)
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(pruned) == 0 {
		return nil, nil
	}

	idx.Workspaces = filtered
	if err := s.saveUnsafe(idx); err != nil {
		return nil, err
	}
	return pruned, nil
}

// GlobalWorkspacePath returns the path where a global workspace would be stored.
func (s *Store) GlobalWorkspacePath(uuid, tag string) string {
	return filepath.Join(s.indexDir, "workspaces", uuid, tag)
}
