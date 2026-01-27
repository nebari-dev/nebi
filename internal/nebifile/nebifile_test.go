package nebifile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestReadNonExistentFile(t *testing.T) {
	_, err := Read("/nonexistent/dir")
	if err == nil {
		t.Fatal("Read() should return error for nonexistent directory")
	}
}

func TestReadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	os.WriteFile(path, []byte("not valid toml = {{{"), 0644)

	_, err := Read(dir)
	if err == nil {
		t.Fatal("Read() should return error for invalid TOML")
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()

	nf := &NebiFile{
		ID: "test-uuid-123",
		Origin: Origin{
			ServerID:    "server-uuid",
			ServerURL:   "https://nebi.example.com",
			SpecID:      "spec-uuid",
			SpecName:    "data-science",
			VersionID:   "version-uuid",
			VersionName: "v1.0",
		},
	}

	if err := Write(dir, nf); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Verify fields
	if loaded.ID != "test-uuid-123" {
		t.Errorf("ID = %q, want %q", loaded.ID, "test-uuid-123")
	}
	if loaded.Origin.SpecName != "data-science" {
		t.Errorf("SpecName = %q, want %q", loaded.Origin.SpecName, "data-science")
	}
	if loaded.Origin.VersionName != "v1.0" {
		t.Errorf("VersionName = %q, want %q", loaded.Origin.VersionName, "v1.0")
	}
	if loaded.Origin.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", loaded.Origin.ServerURL, "https://nebi.example.com")
	}
	if loaded.Origin.ServerID != "server-uuid" {
		t.Errorf("ServerID = %q, want %q", loaded.Origin.ServerID, "server-uuid")
	}
	if loaded.Origin.SpecID != "spec-uuid" {
		t.Errorf("SpecID = %q, want %q", loaded.Origin.SpecID, "spec-uuid")
	}
	if loaded.Origin.VersionID != "version-uuid" {
		t.Errorf("VersionID = %q, want %q", loaded.Origin.VersionID, "version-uuid")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir) {
		t.Error("Exists() should return false for empty directory")
	}

	// Create .nebi.toml file
	nf := &NebiFile{
		ID: "test-uuid",
		Origin: Origin{
			SpecName: "test",
		},
	}
	Write(dir, nf)

	if !Exists(dir) {
		t.Error("Exists() should return true after writing .nebi.toml file")
	}
}

func TestNew(t *testing.T) {
	origin := Origin{
		SpecName:    "test-ws",
		VersionName: "v1.0",
		ServerURL:   "https://example.com",
		VersionID:   "1",
	}

	nf := New(origin)
	if nf.Origin.SpecName != "test-ws" {
		t.Errorf("SpecName = %q, want %q", nf.Origin.SpecName, "test-ws")
	}
	if nf.ID == "" {
		t.Error("ID should be auto-generated")
	}
}

func TestNewFromPull(t *testing.T) {
	nf := NewFromPull(
		"data-science", "v1.0", "https://nebi.example.com",
		"spec-uuid", "version-uuid", "server-uuid",
	)

	if nf.Origin.SpecName != "data-science" {
		t.Errorf("SpecName = %q, want %q", nf.Origin.SpecName, "data-science")
	}
	if nf.Origin.VersionName != "v1.0" {
		t.Errorf("VersionName = %q, want %q", nf.Origin.VersionName, "v1.0")
	}
	if nf.Origin.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", nf.Origin.ServerURL, "https://nebi.example.com")
	}
	if nf.Origin.SpecID != "spec-uuid" {
		t.Errorf("SpecID = %q, want %q", nf.Origin.SpecID, "spec-uuid")
	}
	if nf.Origin.VersionID != "version-uuid" {
		t.Errorf("VersionID = %q, want %q", nf.Origin.VersionID, "version-uuid")
	}
	if nf.Origin.ServerID != "server-uuid" {
		t.Errorf("ServerID = %q, want %q", nf.Origin.ServerID, "server-uuid")
	}
	if nf.ID == "" {
		t.Error("ID should be auto-generated")
	}
}

func TestTOMLFormat(t *testing.T) {
	dir := t.TempDir()

	nf := &NebiFile{
		ID: "test-uuid",
		Origin: Origin{
			ServerID:    "server-uuid",
			ServerURL:   "https://nebi.example.com",
			SpecID:      "spec-uuid",
			SpecName:    "data-science",
			VersionID:   "version-uuid",
			VersionName: "v1.0",
		},
	}

	Write(dir, nf)

	// Read raw TOML and verify structure
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Check top-level fields
	if _, ok := raw["id"]; !ok {
		t.Error("TOML should have 'id' key")
	}
	if _, ok := raw["origin"]; !ok {
		t.Error("TOML should have 'origin' key")
	}

	// Check origin structure
	origin, ok := raw["origin"].(map[string]interface{})
	if !ok {
		t.Fatal("origin should be a map")
	}
	if origin["spec_name"] != "data-science" {
		t.Errorf("origin.spec_name = %v, want %q", origin["spec_name"], "data-science")
	}
	if origin["server_url"] != "https://nebi.example.com" {
		t.Errorf("origin.server_url = %v, want %q", origin["server_url"], "https://nebi.example.com")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.nebi.toml")

	nf := &NebiFile{
		ID: "test-uuid",
		Origin: Origin{
			SpecName: "test",
		},
	}

	if err := WriteFile(path, nf); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if loaded.Origin.SpecName != "test" {
		t.Errorf("SpecName = %q, want %q", loaded.Origin.SpecName, "test")
	}
}

func TestReadFileNonExistent(t *testing.T) {
	_, err := ReadFile("/nonexistent/path/.nebi.toml")
	if err == nil {
		t.Fatal("ReadFile() should return error for nonexistent file")
	}
}

func TestWriteOverwrite(t *testing.T) {
	dir := t.TempDir()

	nf1 := &NebiFile{
		ID:     "uuid-1",
		Origin: Origin{SpecName: "ws1", VersionName: "v1.0"},
	}
	nf2 := &NebiFile{
		ID:     "uuid-2",
		Origin: Origin{SpecName: "ws2", VersionName: "v2.0"},
	}

	Write(dir, nf1)
	Write(dir, nf2)

	loaded, _ := Read(dir)
	if loaded.Origin.SpecName != "ws2" {
		t.Errorf("SpecName = %q, want %q (should be overwritten)", loaded.Origin.SpecName, "ws2")
	}
	if loaded.Origin.VersionName != "v2.0" {
		t.Errorf("VersionName = %q, want %q (should be overwritten)", loaded.Origin.VersionName, "v2.0")
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
	if FileName != ".nebi.toml" {
		t.Errorf("FileName = %q, want %q", FileName, ".nebi.toml")
	}
}

func TestEmptyOriginFields(t *testing.T) {
	dir := t.TempDir()

	// Only required fields
	nf := &NebiFile{
		ID: "test-uuid",
		Origin: Origin{
			SpecName:    "test",
			VersionName: "v1.0",
			ServerURL:   "https://example.com",
			VersionID:   "1",
		},
	}

	if err := Write(dir, nf); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Optional fields should be empty
	if loaded.Origin.SpecID != "" {
		t.Errorf("SpecID = %q, want empty", loaded.Origin.SpecID)
	}
	if loaded.Origin.ServerID != "" {
		t.Errorf("ServerID = %q, want empty", loaded.Origin.ServerID)
	}
}

func TestRoundTripPreservesData(t *testing.T) {
	dir := t.TempDir()

	original := &NebiFile{
		ID: "original-uuid",
		Origin: Origin{
			ServerID:    "server-uuid-123",
			ServerURL:   "https://nebi.internal.company.com:8460",
			SpecID:      "spec-uuid-456",
			SpecName:    "ml-pipeline",
			VersionID:   "version-uuid-789",
			VersionName: "v2.3.1-beta",
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
	if loaded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", loaded.ID, original.ID)
	}
	if loaded.Origin.SpecName != original.Origin.SpecName {
		t.Errorf("SpecName mismatch: got %q, want %q", loaded.Origin.SpecName, original.Origin.SpecName)
	}
	if loaded.Origin.VersionName != original.Origin.VersionName {
		t.Errorf("VersionName mismatch: got %q, want %q", loaded.Origin.VersionName, original.Origin.VersionName)
	}
	if loaded.Origin.ServerURL != original.Origin.ServerURL {
		t.Errorf("ServerURL mismatch: got %q, want %q", loaded.Origin.ServerURL, original.Origin.ServerURL)
	}
	if loaded.Origin.ServerID != original.Origin.ServerID {
		t.Errorf("ServerID mismatch: got %q, want %q", loaded.Origin.ServerID, original.Origin.ServerID)
	}
	if loaded.Origin.SpecID != original.Origin.SpecID {
		t.Errorf("SpecID mismatch: got %q, want %q", loaded.Origin.SpecID, original.Origin.SpecID)
	}
	if loaded.Origin.VersionID != original.Origin.VersionID {
		t.Errorf("VersionID mismatch: got %q, want %q", loaded.Origin.VersionID, original.Origin.VersionID)
	}
}

func TestAutoGenerateID(t *testing.T) {
	dir := t.TempDir()

	// Write without ID
	nf := &NebiFile{
		Origin: Origin{SpecName: "test"},
	}
	Write(dir, nf)

	// Read should have ID
	loaded, _ := Read(dir)
	if loaded.ID == "" {
		t.Error("ID should be auto-generated when writing")
	}
}
