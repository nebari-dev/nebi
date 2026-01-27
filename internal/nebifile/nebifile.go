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

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

const (
	// FileName is the name of the nebi metadata file.
	FileName = ".nebi.toml"

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
}

// Read reads a .nebi.toml file from the given directory.
func Read(dir string) (*NebiFile, error) {
	tomlPath := filepath.Join(dir, FileName)
	return ReadFile(tomlPath)
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

// Write writes the .nebi.toml file to the given directory.
func Write(dir string, nf *NebiFile) error {
	path := filepath.Join(dir, FileName)
	return WriteFile(path, nf)
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

// Exists checks if a .nebi.toml file exists in the given directory.
func Exists(dir string) bool {
	tomlPath := filepath.Join(dir, FileName)
	_, err := os.Stat(tomlPath)
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
// Note: Layer information is stored in index.json, not in .nebi.toml
// Note: pulled_at timestamp is stored in index.json, not in .nebi.toml
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
		},
	}
}
