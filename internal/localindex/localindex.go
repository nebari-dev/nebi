// Package localindex provides CRUD operations for the local repo index.
//
// The local index is stored at ~/.local/share/nebi/index.json and tracks all
// repos that have been pulled to the local machine, including their
// origin information, layer digests for drift detection, and optional aliases.
package localindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// CurrentVersion is the current schema version of the index file.
	CurrentVersion = 2

	// DefaultIndexDir is the default directory for the index file.
	DefaultIndexDir = ".local/share/nebi"

	// IndexFileName is the name of the index file.
	IndexFileName = "index.json"
)

// Index represents the local repo index.
type Index struct {
	Version int              `json:"version"`
	Entries []Entry          `json:"entries"`
	Aliases map[string]Alias `json:"aliases,omitempty"`
}

// Entry represents a single entry in the index.
type Entry struct {
	// ID is a unique identifier for this local entry
	ID string `json:"id"`

	// Spec identification
	SpecName string `json:"spec_name"`
	SpecID   string `json:"spec_id"`

	// Version identification
	VersionName string `json:"version_name"`
	VersionID   string `json:"version_id"`

	// Server information
	ServerURL string `json:"server_url"`
	ServerID  string `json:"server_id"`

	// Local state
	Path     string    `json:"path"`
	PulledAt time.Time `json:"pulled_at"`

	// Layer digests for drift detection
	Layers map[string]string `json:"layers,omitempty"`
}

// RepoEntry is an alias for Entry for backward compatibility.
// Deprecated: Use Entry instead.
type RepoEntry = Entry

// IsGlobal returns true if this entry is stored in global storage.
// Determined by checking if the path is under ~/.local/share/nebi/repos/
func (e *Entry) IsGlobal() bool {
	homeDir, _ := os.UserHomeDir()
	globalPrefix := filepath.Join(homeDir, DefaultIndexDir, "repos") + string(filepath.Separator)
	return strings.HasPrefix(e.Path, globalPrefix)
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
// Respects NEBI_DATA_DIR env var for overriding the data directory.
func NewStore() *Store {
	if dir := os.Getenv("NEBI_DATA_DIR"); dir != "" {
		return &Store{indexDir: dir}
	}
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
				Version: CurrentVersion,
				Entries: []Entry{},
				Aliases: make(map[string]Alias),
			}, nil
		}
		return nil, fmt.Errorf("failed to read index file: %w", err)
	}

	// First try to unmarshal with current format
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse index file: %w", err)
	}

	// Migration: handle old v1 format with "repos" or "workspaces" keys
	if idx.Entries == nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			// Try "repos" first (v1 format), then "workspaces" (v0 format)
			var entriesData json.RawMessage
			if reposData, ok := raw["repos"]; ok {
				entriesData = reposData
			} else if wsData, ok := raw["workspaces"]; ok {
				entriesData = wsData
			}

			if entriesData != nil {
				var oldEntries []struct {
					Workspace       string            `json:"workspace"`
					Repo            string            `json:"repo"`
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
				if err := json.Unmarshal(entriesData, &oldEntries); err == nil {
					idx.Entries = make([]Entry, len(oldEntries))
					for i, old := range oldEntries {
						// Use Repo if present, fall back to Workspace
						specName := old.Repo
						if specName == "" {
							specName = old.Workspace
						}
						idx.Entries[i] = Entry{
							ID:          uuid.New().String(),
							SpecName:    specName,
							SpecID:      "", // Not available in old format
							VersionName: old.Tag,
							VersionID:   fmt.Sprintf("%d", old.ServerVersionID), // Convert int to string
							ServerURL:   old.ServerURL,
							ServerID:    "", // Not available in old format
							Path:        old.Path,
							PulledAt:    old.PulledAt,
							Layers:      old.Layers,
						}
					}
				}
			}
		}
	}

	if idx.Entries == nil {
		idx.Entries = []Entry{}
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

// AddEntry adds or updates an entry in the index.
// If an entry with the same path already exists, it is replaced.
func (s *Store) AddEntry(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}

	// Ensure entry has an ID
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	// Replace existing entry with same path
	found := false
	for i, existing := range idx.Entries {
		if existing.Path == entry.Path {
			// Preserve the original ID when replacing
			if entry.ID == "" {
				entry.ID = existing.ID
			}
			idx.Entries[i] = entry
			found = true
			break
		}
	}
	if !found {
		idx.Entries = append(idx.Entries, entry)
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
	filtered := make([]Entry, 0, len(idx.Entries))
	for _, entry := range idx.Entries {
		if entry.Path == path {
			found = true
			continue
		}
		filtered = append(filtered, entry)
	}

	if !found {
		return false, nil
	}

	idx.Entries = filtered
	return true, s.saveUnsafe(idx)
}

// FindByPath returns the entry at the given path, or nil if not found.
func (s *Store) FindByPath(path string) (*Entry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	for i := range idx.Entries {
		if idx.Entries[i].Path == path {
			return &idx.Entries[i], nil
		}
	}
	return nil, nil
}

// FindByID returns the entry with the given ID, or nil if not found.
func (s *Store) FindByID(id string) (*Entry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	for i := range idx.Entries {
		if idx.Entries[i].ID == id {
			return &idx.Entries[i], nil
		}
	}
	return nil, nil
}

// FindBySpecVersion returns all entries matching a spec name and version name.
func (s *Store) FindBySpecVersion(specName, versionName string) ([]Entry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	var matches []Entry
	for _, entry := range idx.Entries {
		if entry.SpecName == specName && entry.VersionName == versionName {
			matches = append(matches, entry)
		}
	}
	return matches, nil
}

// FindByRepoTag is an alias for FindBySpecVersion for backward compatibility.
// Deprecated: Use FindBySpecVersion instead.
func (s *Store) FindByRepoTag(repo, tag string) ([]Entry, error) {
	return s.FindBySpecVersion(repo, tag)
}

// FindGlobal returns the global entry matching a spec name and version name.
// Global entries are identified by having a path under the global repos directory.
func (s *Store) FindGlobal(specName, versionName string) (*Entry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}

	globalPrefix := s.indexDir + "/repos/"
	for i := range idx.Entries {
		if idx.Entries[i].SpecName == specName &&
			idx.Entries[i].VersionName == versionName &&
			len(idx.Entries[i].Path) > len(globalPrefix) &&
			idx.Entries[i].Path[:len(globalPrefix)] == globalPrefix {
			return &idx.Entries[i], nil
		}
	}
	return nil, nil
}

// IsGlobal returns true if the entry is stored in global storage.
func (s *Store) IsGlobal(entry *Entry) bool {
	globalPrefix := s.indexDir + "/repos/"
	return len(entry.Path) > len(globalPrefix) && entry.Path[:len(globalPrefix)] == globalPrefix
}

// ListAll returns all entries.
func (s *Store) ListAll() ([]Entry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}
	return idx.Entries, nil
}

// SetAlias sets a user-friendly alias for a global repo.
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
func (s *Store) Prune() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}

	var pruned []Entry
	filtered := make([]Entry, 0, len(idx.Entries))
	for _, entry := range idx.Entries {
		if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
			pruned = append(pruned, entry)
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(pruned) == 0 {
		return nil, nil
	}

	idx.Entries = filtered
	if err := s.saveUnsafe(idx); err != nil {
		return nil, err
	}
	return pruned, nil
}

// GlobalRepoPath returns the path where a global repo would be stored.
func (s *Store) GlobalRepoPath(uuid, tag string) string {
	return filepath.Join(s.indexDir, "repos", uuid, tag)
}
