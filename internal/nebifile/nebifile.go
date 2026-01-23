// Package nebifile provides read/write operations for .nebi metadata files.
//
// A .nebi file is written to a workspace directory after a pull operation.
// It contains origin information (where the workspace was pulled from) and
// per-file layer digests for drift detection.
//
// Format (YAML):
//
//	origin:
//	  workspace: data-science
//	  tag: v1.0
//	  registry: ds-team
//	  server_url: https://nebi.example.com
//	  server_version_id: 42
//	  manifest_digest: sha256:abc123...
//	  pulled_at: 2024-01-20T10:30:00Z
//	layers:
//	  pixi.toml:
//	    digest: sha256:111...
//	    size: 2345
//	    media_type: application/vnd.pixi.toml.v1+toml
//	  pixi.lock:
//	    digest: sha256:222...
//	    size: 45678
//	    media_type: application/vnd.pixi.lock.v1+yaml
package nebifile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// FileName is the name of the nebi metadata file.
	FileName = ".nebi"

	// MediaTypePixiToml is the OCI media type for pixi.toml files.
	MediaTypePixiToml = "application/vnd.pixi.toml.v1+toml"

	// MediaTypePixiLock is the OCI media type for pixi.lock files.
	MediaTypePixiLock = "application/vnd.pixi.lock.v1+yaml"
)

// NebiFile represents the contents of a .nebi metadata file.
type NebiFile struct {
	Origin Origin           `yaml:"origin"`
	Layers map[string]Layer `yaml:"layers"`
}

// Origin contains information about where the workspace was pulled from.
type Origin struct {
	Workspace       string    `yaml:"workspace"`
	Tag             string    `yaml:"tag"`
	RegistryURL     string    `yaml:"registry_url,omitempty"`
	ServerURL       string    `yaml:"server_url"`
	ServerVersionID int32     `yaml:"server_version_id"`
	ManifestDigest  string    `yaml:"manifest_digest,omitempty"`
	PulledAt        time.Time `yaml:"pulled_at"`
}

// Layer contains information about a single file layer.
type Layer struct {
	Digest    string `yaml:"digest"`
	Size      int64  `yaml:"size"`
	MediaType string `yaml:"media_type"`
}

// Read reads a .nebi file from the given directory.
func Read(dir string) (*NebiFile, error) {
	path := filepath.Join(dir, FileName)
	return ReadFile(path)
}

// ReadFile reads a .nebi file from the given path.
func ReadFile(path string) (*NebiFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not a nebi workspace: %s not found", path)
		}
		return nil, fmt.Errorf("failed to read %s: %w", FileName, err)
	}

	var nf NebiFile
	if err := yaml.Unmarshal(data, &nf); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", FileName, err)
	}

	if nf.Layers == nil {
		nf.Layers = make(map[string]Layer)
	}

	return &nf, nil
}

// Write writes the .nebi file to the given directory.
func Write(dir string, nf *NebiFile) error {
	path := filepath.Join(dir, FileName)
	return WriteFile(path, nf)
}

// WriteFile writes the .nebi file to the given path.
func WriteFile(path string, nf *NebiFile) error {
	data, err := yaml.Marshal(nf)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", FileName, err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", FileName, err)
	}

	return nil
}

// Exists checks if a .nebi file exists in the given directory.
func Exists(dir string) bool {
	path := filepath.Join(dir, FileName)
	_, err := os.Stat(path)
	return err == nil
}

// New creates a new NebiFile with the given origin and layer information.
func New(origin Origin, layers map[string]Layer) *NebiFile {
	if layers == nil {
		layers = make(map[string]Layer)
	}
	return &NebiFile{
		Origin: origin,
		Layers: layers,
	}
}

// NewFromPull creates a NebiFile from pull operation results.
// This is a convenience constructor that takes the common parameters from a pull.
func NewFromPull(workspace, tag, registryURL, serverURL string, serverVersionID int32,
	manifestDigest string, pixiTomlDigest string, pixiTomlSize int64,
	pixiLockDigest string, pixiLockSize int64) *NebiFile {

	return &NebiFile{
		Origin: Origin{
			Workspace:       workspace,
			Tag:             tag,
			RegistryURL:     registryURL,
			ServerURL:       serverURL,
			ServerVersionID: serverVersionID,
			ManifestDigest:  manifestDigest,
			PulledAt:        time.Now(),
		},
		Layers: map[string]Layer{
			"pixi.toml": {
				Digest:    pixiTomlDigest,
				Size:      pixiTomlSize,
				MediaType: MediaTypePixiToml,
			},
			"pixi.lock": {
				Digest:    pixiLockDigest,
				Size:      pixiLockSize,
				MediaType: MediaTypePixiLock,
			},
		},
	}
}

// GetLayerDigest returns the digest for a specific file layer.
// Returns empty string if the layer is not found.
func (nf *NebiFile) GetLayerDigest(filename string) string {
	if layer, ok := nf.Layers[filename]; ok {
		return layer.Digest
	}
	return ""
}

// HasLayer checks if the file has a layer entry for the given filename.
func (nf *NebiFile) HasLayer(filename string) bool {
	_, ok := nf.Layers[filename]
	return ok
}
