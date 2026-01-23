package nebifile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestReadNonExistentFile(t *testing.T) {
	_, err := Read("/nonexistent/dir")
	if err == nil {
		t.Fatal("Read() should return error for nonexistent directory")
	}
}

func TestReadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	os.WriteFile(path, []byte("not: valid: yaml: {{{"), 0644)

	_, err := Read(dir)
	if err == nil {
		t.Fatal("Read() should return error for invalid YAML")
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2024, 1, 20, 10, 30, 0, 0, time.UTC)

	nf := &NebiFile{
		Origin: Origin{
			Workspace:       "data-science",
			Tag:             "v1.0",
			RegistryURL:        "ds-team",
			ServerURL:       "https://nebi.example.com",
			ServerVersionID: 42,
			ManifestDigest:  "sha256:abc123def456",
			PulledAt:        now,
		},
		Layers: map[string]Layer{
			"pixi.toml": {
				Digest:    "sha256:111aaa",
				Size:      2345,
				MediaType: MediaTypePixiToml,
			},
			"pixi.lock": {
				Digest:    "sha256:222bbb",
				Size:      45678,
				MediaType: MediaTypePixiLock,
			},
		},
	}

	if err := Write(dir, nf); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Verify origin
	if loaded.Origin.Workspace != "data-science" {
		t.Errorf("Workspace = %q, want %q", loaded.Origin.Workspace, "data-science")
	}
	if loaded.Origin.Tag != "v1.0" {
		t.Errorf("Tag = %q, want %q", loaded.Origin.Tag, "v1.0")
	}
	if loaded.Origin.RegistryURL != "ds-team" {
		t.Errorf("Registry = %q, want %q", loaded.Origin.RegistryURL, "ds-team")
	}
	if loaded.Origin.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", loaded.Origin.ServerURL, "https://nebi.example.com")
	}
	if loaded.Origin.ServerVersionID != 42 {
		t.Errorf("ServerVersionID = %d, want 42", loaded.Origin.ServerVersionID)
	}
	if loaded.Origin.ManifestDigest != "sha256:abc123def456" {
		t.Errorf("ManifestDigest = %q, want %q", loaded.Origin.ManifestDigest, "sha256:abc123def456")
	}
	if !loaded.Origin.PulledAt.Equal(now) {
		t.Errorf("PulledAt = %v, want %v", loaded.Origin.PulledAt, now)
	}

	// Verify layers
	if len(loaded.Layers) != 2 {
		t.Fatalf("Layers length = %d, want 2", len(loaded.Layers))
	}

	tomlLayer := loaded.Layers["pixi.toml"]
	if tomlLayer.Digest != "sha256:111aaa" {
		t.Errorf("pixi.toml Digest = %q, want %q", tomlLayer.Digest, "sha256:111aaa")
	}
	if tomlLayer.Size != 2345 {
		t.Errorf("pixi.toml Size = %d, want 2345", tomlLayer.Size)
	}
	if tomlLayer.MediaType != MediaTypePixiToml {
		t.Errorf("pixi.toml MediaType = %q, want %q", tomlLayer.MediaType, MediaTypePixiToml)
	}

	lockLayer := loaded.Layers["pixi.lock"]
	if lockLayer.Digest != "sha256:222bbb" {
		t.Errorf("pixi.lock Digest = %q, want %q", lockLayer.Digest, "sha256:222bbb")
	}
	if lockLayer.Size != 45678 {
		t.Errorf("pixi.lock Size = %d, want 45678", lockLayer.Size)
	}
	if lockLayer.MediaType != MediaTypePixiLock {
		t.Errorf("pixi.lock MediaType = %q, want %q", lockLayer.MediaType, MediaTypePixiLock)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir) {
		t.Error("Exists() should return false for empty directory")
	}

	// Create .nebi file
	nf := &NebiFile{
		Origin: Origin{Workspace: "test"},
		Layers: make(map[string]Layer),
	}
	Write(dir, nf)

	if !Exists(dir) {
		t.Error("Exists() should return true after writing .nebi file")
	}
}

func TestNew(t *testing.T) {
	origin := Origin{
		Workspace:       "test-ws",
		Tag:             "v1.0",
		ServerURL:       "https://example.com",
		ServerVersionID: 1,
	}
	layers := map[string]Layer{
		"pixi.toml": {Digest: "sha256:aaa", Size: 100, MediaType: MediaTypePixiToml},
	}

	nf := New(origin, layers)
	if nf.Origin.Workspace != "test-ws" {
		t.Errorf("Workspace = %q, want %q", nf.Origin.Workspace, "test-ws")
	}
	if len(nf.Layers) != 1 {
		t.Errorf("Layers length = %d, want 1", len(nf.Layers))
	}
}

func TestNewNilLayers(t *testing.T) {
	origin := Origin{Workspace: "test-ws"}
	nf := New(origin, nil)
	if nf.Layers == nil {
		t.Error("Layers should not be nil when created with nil argument")
	}
}

func TestNewFromPull(t *testing.T) {
	nf := NewFromPull(
		"data-science", "v1.0", "ds-team", "https://nebi.example.com",
		42, "sha256:manifest123",
		"sha256:toml456", 2345,
		"sha256:lock789", 45678,
	)

	if nf.Origin.Workspace != "data-science" {
		t.Errorf("Workspace = %q, want %q", nf.Origin.Workspace, "data-science")
	}
	if nf.Origin.Tag != "v1.0" {
		t.Errorf("Tag = %q, want %q", nf.Origin.Tag, "v1.0")
	}
	if nf.Origin.RegistryURL != "ds-team" {
		t.Errorf("Registry = %q, want %q", nf.Origin.RegistryURL, "ds-team")
	}
	if nf.Origin.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", nf.Origin.ServerURL, "https://nebi.example.com")
	}
	if nf.Origin.ServerVersionID != 42 {
		t.Errorf("ServerVersionID = %d, want 42", nf.Origin.ServerVersionID)
	}
	if nf.Origin.ManifestDigest != "sha256:manifest123" {
		t.Errorf("ManifestDigest = %q, want %q", nf.Origin.ManifestDigest, "sha256:manifest123")
	}
	if nf.Origin.PulledAt.IsZero() {
		t.Error("PulledAt should not be zero")
	}

	toml := nf.Layers["pixi.toml"]
	if toml.Digest != "sha256:toml456" {
		t.Errorf("pixi.toml Digest = %q, want %q", toml.Digest, "sha256:toml456")
	}
	if toml.Size != 2345 {
		t.Errorf("pixi.toml Size = %d, want 2345", toml.Size)
	}
	if toml.MediaType != MediaTypePixiToml {
		t.Errorf("pixi.toml MediaType = %q, want %q", toml.MediaType, MediaTypePixiToml)
	}

	lock := nf.Layers["pixi.lock"]
	if lock.Digest != "sha256:lock789" {
		t.Errorf("pixi.lock Digest = %q, want %q", lock.Digest, "sha256:lock789")
	}
	if lock.Size != 45678 {
		t.Errorf("pixi.lock Size = %d, want 45678", lock.Size)
	}
	if lock.MediaType != MediaTypePixiLock {
		t.Errorf("pixi.lock MediaType = %q, want %q", lock.MediaType, MediaTypePixiLock)
	}
}

func TestGetLayerDigest(t *testing.T) {
	nf := &NebiFile{
		Layers: map[string]Layer{
			"pixi.toml": {Digest: "sha256:aaa"},
			"pixi.lock": {Digest: "sha256:bbb"},
		},
	}

	if got := nf.GetLayerDigest("pixi.toml"); got != "sha256:aaa" {
		t.Errorf("GetLayerDigest(pixi.toml) = %q, want %q", got, "sha256:aaa")
	}
	if got := nf.GetLayerDigest("pixi.lock"); got != "sha256:bbb" {
		t.Errorf("GetLayerDigest(pixi.lock) = %q, want %q", got, "sha256:bbb")
	}
	if got := nf.GetLayerDigest("nonexistent"); got != "" {
		t.Errorf("GetLayerDigest(nonexistent) = %q, want empty string", got)
	}
}

func TestHasLayer(t *testing.T) {
	nf := &NebiFile{
		Layers: map[string]Layer{
			"pixi.toml": {Digest: "sha256:aaa"},
		},
	}

	if !nf.HasLayer("pixi.toml") {
		t.Error("HasLayer(pixi.toml) should return true")
	}
	if nf.HasLayer("pixi.lock") {
		t.Error("HasLayer(pixi.lock) should return false")
	}
}

func TestYAMLFormat(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2024, 1, 20, 10, 30, 0, 0, time.UTC)

	nf := &NebiFile{
		Origin: Origin{
			Workspace:       "data-science",
			Tag:             "v1.0",
			RegistryURL:        "ds-team",
			ServerURL:       "https://nebi.example.com",
			ServerVersionID: 42,
			ManifestDigest:  "sha256:abc123",
			PulledAt:        now,
		},
		Layers: map[string]Layer{
			"pixi.toml": {
				Digest:    "sha256:111",
				Size:      2345,
				MediaType: MediaTypePixiToml,
			},
		},
	}

	Write(dir, nf)

	// Read raw YAML and verify structure
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Check top-level keys
	if _, ok := raw["origin"]; !ok {
		t.Error("YAML should have 'origin' key")
	}
	if _, ok := raw["layers"]; !ok {
		t.Error("YAML should have 'layers' key")
	}

	// Check origin structure
	origin, ok := raw["origin"].(map[string]interface{})
	if !ok {
		t.Fatal("origin should be a map")
	}
	if origin["workspace"] != "data-science" {
		t.Errorf("origin.workspace = %v, want %q", origin["workspace"], "data-science")
	}
	if origin["server_url"] != "https://nebi.example.com" {
		t.Errorf("origin.server_url = %v, want %q", origin["server_url"], "https://nebi.example.com")
	}
}

func TestReadNilLayers(t *testing.T) {
	dir := t.TempDir()

	// Write YAML without layers field
	data := `origin:
  workspace: test
  tag: v1.0
  server_url: https://example.com
  server_version_id: 1
  pulled_at: 2024-01-20T10:30:00Z
`
	os.WriteFile(filepath.Join(dir, FileName), []byte(data), 0644)

	nf, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if nf.Layers == nil {
		t.Error("Layers should be initialized to empty map, not nil")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.nebi")

	nf := &NebiFile{
		Origin: Origin{Workspace: "test"},
		Layers: make(map[string]Layer),
	}

	if err := WriteFile(path, nf); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if loaded.Origin.Workspace != "test" {
		t.Errorf("Workspace = %q, want %q", loaded.Origin.Workspace, "test")
	}
}

func TestReadFileNonExistent(t *testing.T) {
	_, err := ReadFile("/nonexistent/path/.nebi")
	if err == nil {
		t.Fatal("ReadFile() should return error for nonexistent file")
	}
}

func TestWriteOverwrite(t *testing.T) {
	dir := t.TempDir()

	nf1 := &NebiFile{
		Origin: Origin{Workspace: "ws1", Tag: "v1.0"},
		Layers: make(map[string]Layer),
	}
	nf2 := &NebiFile{
		Origin: Origin{Workspace: "ws2", Tag: "v2.0"},
		Layers: make(map[string]Layer),
	}

	Write(dir, nf1)
	Write(dir, nf2)

	loaded, _ := Read(dir)
	if loaded.Origin.Workspace != "ws2" {
		t.Errorf("Workspace = %q, want %q (should be overwritten)", loaded.Origin.Workspace, "ws2")
	}
	if loaded.Origin.Tag != "v2.0" {
		t.Errorf("Tag = %q, want %q (should be overwritten)", loaded.Origin.Tag, "v2.0")
	}
}

func TestMediaTypeConstants(t *testing.T) {
	if MediaTypePixiToml != "application/vnd.pixi.toml.v1+toml" {
		t.Errorf("MediaTypePixiToml = %q, unexpected value", MediaTypePixiToml)
	}
	if MediaTypePixiLock != "application/vnd.pixi.lock.v1+yaml" {
		t.Errorf("MediaTypePixiLock = %q, unexpected value", MediaTypePixiLock)
	}
}

func TestFileNameConstant(t *testing.T) {
	if FileName != ".nebi" {
		t.Errorf("FileName = %q, want %q", FileName, ".nebi")
	}
}

func TestEmptyOriginFields(t *testing.T) {
	dir := t.TempDir()

	// Only required fields
	nf := &NebiFile{
		Origin: Origin{
			Workspace:       "test",
			Tag:             "v1.0",
			ServerURL:       "https://example.com",
			ServerVersionID: 1,
			PulledAt:        time.Now(),
		},
		Layers: make(map[string]Layer),
	}

	if err := Write(dir, nf); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Optional fields should be empty
	if loaded.Origin.RegistryURL != "" {
		t.Errorf("Registry = %q, want empty", loaded.Origin.RegistryURL)
	}
	if loaded.Origin.ManifestDigest != "" {
		t.Errorf("ManifestDigest = %q, want empty", loaded.Origin.ManifestDigest)
	}
}

func TestRoundTripPreservesData(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)

	original := &NebiFile{
		Origin: Origin{
			Workspace:       "ml-pipeline",
			Tag:             "v2.3.1-beta",
			RegistryURL:        "ml-team",
			ServerURL:       "https://nebi.internal.company.com:8460",
			ServerVersionID: 127,
			ManifestDigest:  "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			PulledAt:        now,
		},
		Layers: map[string]Layer{
			"pixi.toml": {
				Digest:    "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				Size:      4096,
				MediaType: MediaTypePixiToml,
			},
			"pixi.lock": {
				Digest:    "sha256:f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5",
				Size:      102400,
				MediaType: MediaTypePixiLock,
			},
		},
	}

	if err := Write(dir, original); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Compare all fields
	if loaded.Origin.Workspace != original.Origin.Workspace {
		t.Errorf("Workspace mismatch: got %q, want %q", loaded.Origin.Workspace, original.Origin.Workspace)
	}
	if loaded.Origin.Tag != original.Origin.Tag {
		t.Errorf("Tag mismatch: got %q, want %q", loaded.Origin.Tag, original.Origin.Tag)
	}
	if loaded.Origin.RegistryURL != original.Origin.RegistryURL {
		t.Errorf("Registry mismatch: got %q, want %q", loaded.Origin.RegistryURL, original.Origin.RegistryURL)
	}
	if loaded.Origin.ServerURL != original.Origin.ServerURL {
		t.Errorf("ServerURL mismatch: got %q, want %q", loaded.Origin.ServerURL, original.Origin.ServerURL)
	}
	if loaded.Origin.ServerVersionID != original.Origin.ServerVersionID {
		t.Errorf("ServerVersionID mismatch: got %d, want %d", loaded.Origin.ServerVersionID, original.Origin.ServerVersionID)
	}
	if loaded.Origin.ManifestDigest != original.Origin.ManifestDigest {
		t.Errorf("ManifestDigest mismatch: got %q, want %q", loaded.Origin.ManifestDigest, original.Origin.ManifestDigest)
	}
	if !loaded.Origin.PulledAt.Equal(original.Origin.PulledAt) {
		t.Errorf("PulledAt mismatch: got %v, want %v", loaded.Origin.PulledAt, original.Origin.PulledAt)
	}

	for name, origLayer := range original.Layers {
		loadedLayer, ok := loaded.Layers[name]
		if !ok {
			t.Errorf("Layer %q not found in loaded file", name)
			continue
		}
		if loadedLayer.Digest != origLayer.Digest {
			t.Errorf("Layer %q Digest mismatch: got %q, want %q", name, loadedLayer.Digest, origLayer.Digest)
		}
		if loadedLayer.Size != origLayer.Size {
			t.Errorf("Layer %q Size mismatch: got %d, want %d", name, loadedLayer.Size, origLayer.Size)
		}
		if loadedLayer.MediaType != origLayer.MediaType {
			t.Errorf("Layer %q MediaType mismatch: got %q, want %q", name, loadedLayer.MediaType, origLayer.MediaType)
		}
	}
}
