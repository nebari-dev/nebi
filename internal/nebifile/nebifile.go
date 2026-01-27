// Package nebifile provides read/write operations for .nebi.toml metadata files.
//
// A .nebi.toml file is written to a repo directory after a pull operation.
// It contains origin information (where the repo was pulled from).
//
// Format (TOML):
//
//	id = "550e8400-e29b-41d4-a716-446655440000"
//
//	[origin]
//	server_id = "s9t0u1v2-..."
//	server_url = "http://localhost:8460"
//
//	spec_id = "a1b2c3d4-..."
//	spec_name = "data-science"
//
//	version_id = "e5f6g7h8-..."
//	version_name = "v1"
package nebifile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

const (
	// FileName is the name of the nebi metadata file (new TOML format).
	FileName = ".nebi.toml"

	// OldFileName is the name of the old YAML format file.
	OldFileName = ".nebi"

	// MediaTypePixiToml is the OCI media type for pixi.toml files.
	MediaTypePixiToml = "application/vnd.pixi.toml.v1+toml"

	// MediaTypePixiLock is the OCI media type for pixi.lock files.
	MediaTypePixiLock = "application/vnd.pixi.lock.v1+yaml"
)

// NebiFile represents the contents of a .nebi.toml metadata file.
type NebiFile struct {
	// ID is a unique identifier for this local instance
	ID string `toml:"id"`

	// Origin contains information about where the spec was pulled from
	Origin Origin `toml:"origin"`
}

// Origin contains information about where the spec was pulled from.
type Origin struct {
	// Server information
	ServerID  string `toml:"server_id"`
	ServerURL string `toml:"server_url"`

	// Spec identification
	SpecID   string `toml:"spec_id"`
	SpecName string `toml:"spec_name"`

	// Version identification
	VersionID   string `toml:"version_id"`
	VersionName string `toml:"version_name"`

	// Timestamp of when pulled
	PulledAt time.Time `toml:"pulled_at"`
}

// Layer contains information about a single file layer.
// Note: Layer info is now only stored in index.json, not in .nebi.toml
type Layer struct {
	Digest    string `yaml:"digest"`
	Size      int64  `yaml:"size"`
	MediaType string `yaml:"media_type"`
}

// Read reads a .nebi.toml file from the given directory.
// Falls back to reading old .nebi YAML format for migration.
func Read(dir string) (*NebiFile, error) {
	// Try new TOML format first
	tomlPath := filepath.Join(dir, FileName)
	if _, err := os.Stat(tomlPath); err == nil {
		return ReadFile(tomlPath)
	}

	// Fall back to old YAML format
	yamlPath := filepath.Join(dir, OldFileName)
	return readOldYAMLFile(yamlPath)
}

// ReadFile reads a .nebi.toml file from the given path.
func ReadFile(path string) (*NebiFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not a nebi repo: %s not found", path)
		}
		return nil, fmt.Errorf("failed to read %s: %w", FileName, err)
	}

	var nf NebiFile
	if err := toml.Unmarshal(data, &nf); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", FileName, err)
	}

	// Ensure ID is set
	if nf.ID == "" {
		nf.ID = uuid.New().String()
	}

	return &nf, nil
}

// readOldYAMLFile reads the old .nebi YAML format and converts to new format.
func readOldYAMLFile(path string) (*NebiFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not a nebi repo: %s not found", path)
		}
		return nil, fmt.Errorf("failed to read %s: %w", OldFileName, err)
	}

	// Parse old YAML format
	var oldFormat struct {
		Origin struct {
			Workspace       string    `yaml:"workspace"`
			Repo            string    `yaml:"repo"`
			Tag             string    `yaml:"tag"`
			RegistryURL     string    `yaml:"registry_url,omitempty"`
			ServerURL       string    `yaml:"server_url"`
			ServerVersionID int32     `yaml:"server_version_id"`
			ManifestDigest  string    `yaml:"manifest_digest,omitempty"`
			PulledAt        time.Time `yaml:"pulled_at"`
		} `yaml:"origin"`
		Layers map[string]Layer `yaml:"layers"`
	}
	if err := yaml.Unmarshal(data, &oldFormat); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", OldFileName, err)
	}

	// Convert to new format
	specName := oldFormat.Origin.Repo
	if specName == "" {
		specName = oldFormat.Origin.Workspace
	}

	return &NebiFile{
		ID: uuid.New().String(),
		Origin: Origin{
			ServerID:    "", // Not available in old format
			ServerURL:   oldFormat.Origin.ServerURL,
			SpecID:      "", // Not available in old format
			SpecName:    specName,
			VersionID:   fmt.Sprintf("%d", oldFormat.Origin.ServerVersionID),
			VersionName: oldFormat.Origin.Tag,
			PulledAt:    oldFormat.Origin.PulledAt,
		},
	}, nil
}

// Write writes the .nebi.toml file to the given directory.
// Also removes old .nebi file if it exists.
func Write(dir string, nf *NebiFile) error {
	path := filepath.Join(dir, FileName)
	if err := WriteFile(path, nf); err != nil {
		return err
	}

	// Clean up old YAML file if it exists
	oldPath := filepath.Join(dir, OldFileName)
	os.Remove(oldPath) // Ignore error - file may not exist

	return nil
}

// WriteFile writes the .nebi.toml file to the given path.
func WriteFile(path string, nf *NebiFile) error {
	// Ensure ID is set
	if nf.ID == "" {
		nf.ID = uuid.New().String()
	}

	data, err := toml.Marshal(nf)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", FileName, err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", FileName, err)
	}

	return nil
}

// Exists checks if a .nebi.toml or .nebi file exists in the given directory.
func Exists(dir string) bool {
	// Check new TOML format first
	tomlPath := filepath.Join(dir, FileName)
	if _, err := os.Stat(tomlPath); err == nil {
		return true
	}

	// Check old YAML format
	yamlPath := filepath.Join(dir, OldFileName)
	_, err := os.Stat(yamlPath)
	return err == nil
}

// New creates a new NebiFile with the given origin.
func New(origin Origin) *NebiFile {
	return &NebiFile{
		ID:     uuid.New().String(),
		Origin: origin,
	}
}

// NewFromPull creates a NebiFile from pull operation results.
// This is a convenience constructor that takes the common parameters from a pull.
// Note: Layer information is no longer stored in .nebi.toml - it stays in index.json
func NewFromPull(specName, versionName, serverURL, specID, versionID, serverID string) *NebiFile {
	return &NebiFile{
		ID: uuid.New().String(),
		Origin: Origin{
			ServerID:    serverID,
			ServerURL:   serverURL,
			SpecID:      specID,
			SpecName:    specName,
			VersionID:   versionID,
			VersionName: versionName,
			PulledAt:    time.Now(),
		},
	}
}
