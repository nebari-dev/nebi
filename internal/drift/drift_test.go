package drift

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aktech/darb/internal/nebifile"
)

func setupTestWorkspace(t *testing.T, pixiToml, pixiLock string) (string, *nebifile.NebiFile) {
	t.Helper()
	dir := t.TempDir()

	// Write files
	tomlBytes := []byte(pixiToml)
	lockBytes := []byte(pixiLock)
	os.WriteFile(filepath.Join(dir, "pixi.toml"), tomlBytes, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), lockBytes, 0644)

	// Compute digests
	tomlDigest := nebifile.ComputeDigest(tomlBytes)
	lockDigest := nebifile.ComputeDigest(lockBytes)

	// Create .nebi file
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "test-registry", "https://nebi.example.com",
		1, "sha256:manifest123",
		tomlDigest, int64(len(tomlBytes)),
		lockDigest, int64(len(lockBytes)),
	)
	nebifile.Write(dir, nf)

	return dir, nf
}

func TestCheck_Clean(t *testing.T) {
	dir, _ := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	ws, err := Check(dir)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if ws.Overall != StatusClean {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusClean)
	}
	if ws.IsModified() {
		t.Error("IsModified() should be false for clean workspace")
	}
}

func TestCheck_ModifiedPixiToml(t *testing.T) {
	dir, _ := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	// Modify pixi.toml
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[workspace]\nname = \"modified\"\n"), 0644)

	ws, err := Check(dir)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if ws.Overall != StatusModified {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusModified)
	}
	if !ws.IsModified() {
		t.Error("IsModified() should be true for modified workspace")
	}

	// Check file-level status
	tomlStatus := ws.GetFileStatus("pixi.toml")
	if tomlStatus == nil {
		t.Fatal("GetFileStatus(pixi.toml) returned nil")
	}
	if tomlStatus.Status != StatusModified {
		t.Errorf("pixi.toml status = %q, want %q", tomlStatus.Status, StatusModified)
	}

	lockStatus := ws.GetFileStatus("pixi.lock")
	if lockStatus == nil {
		t.Fatal("GetFileStatus(pixi.lock) returned nil")
	}
	if lockStatus.Status != StatusClean {
		t.Errorf("pixi.lock status = %q, want %q", lockStatus.Status, StatusClean)
	}
}

func TestCheck_ModifiedPixiLock(t *testing.T) {
	dir, _ := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	// Modify pixi.lock
	os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte("version: 2\npackages:\n  - numpy\n"), 0644)

	ws, err := Check(dir)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if ws.Overall != StatusModified {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusModified)
	}

	lockStatus := ws.GetFileStatus("pixi.lock")
	if lockStatus.Status != StatusModified {
		t.Errorf("pixi.lock status = %q, want %q", lockStatus.Status, StatusModified)
	}
}

func TestCheck_BothModified(t *testing.T) {
	dir, _ := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	// Modify both files
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("modified toml"), 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte("modified lock"), 0644)

	ws, err := Check(dir)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if ws.Overall != StatusModified {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusModified)
	}

	tomlStatus := ws.GetFileStatus("pixi.toml")
	if tomlStatus.Status != StatusModified {
		t.Errorf("pixi.toml status = %q, want %q", tomlStatus.Status, StatusModified)
	}

	lockStatus := ws.GetFileStatus("pixi.lock")
	if lockStatus.Status != StatusModified {
		t.Errorf("pixi.lock status = %q, want %q", lockStatus.Status, StatusModified)
	}
}

func TestCheck_MissingFile(t *testing.T) {
	dir, _ := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	// Delete pixi.lock
	os.Remove(filepath.Join(dir, "pixi.lock"))

	ws, err := Check(dir)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if ws.Overall != StatusMissing {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusMissing)
	}
	if !ws.IsModified() {
		t.Error("IsModified() should be true when files are missing")
	}

	lockStatus := ws.GetFileStatus("pixi.lock")
	if lockStatus.Status != StatusMissing {
		t.Errorf("pixi.lock status = %q, want %q", lockStatus.Status, StatusMissing)
	}
}

func TestCheck_MixedModifiedAndMissing(t *testing.T) {
	dir, _ := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	// Modify pixi.toml, delete pixi.lock
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("changed content"), 0644)
	os.Remove(filepath.Join(dir, "pixi.lock"))

	ws, err := Check(dir)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	// Missing takes precedence over modified
	if ws.Overall != StatusMissing {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusMissing)
	}

	tomlStatus := ws.GetFileStatus("pixi.toml")
	if tomlStatus.Status != StatusModified {
		t.Errorf("pixi.toml status = %q, want %q", tomlStatus.Status, StatusModified)
	}

	lockStatus := ws.GetFileStatus("pixi.lock")
	if lockStatus.Status != StatusMissing {
		t.Errorf("pixi.lock status = %q, want %q", lockStatus.Status, StatusMissing)
	}
}

func TestIsModified_TrueForMissing(t *testing.T) {
	ws := &WorkspaceStatus{Overall: StatusMissing}
	if !ws.IsModified() {
		t.Error("IsModified() should be true when overall is StatusMissing")
	}
}

func TestCheck_NoNebiFile(t *testing.T) {
	dir := t.TempDir()

	ws, err := Check(dir)
	if err == nil {
		t.Fatal("Check() should return error when .nebi file is missing")
	}
	if ws.Overall != StatusUnknown {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusUnknown)
	}
}

func TestCheckWithNebiFile(t *testing.T) {
	dir := t.TempDir()

	content := []byte("test content")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), content, 0644)

	digest := nebifile.ComputeDigest(content)
	nf := &nebifile.NebiFile{
		Layers: map[string]nebifile.Layer{
			"pixi.toml": {Digest: digest, Size: int64(len(content))},
		},
	}

	ws := CheckWithNebiFile(dir, nf)
	if ws.Overall != StatusClean {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusClean)
	}
}

func TestGetFileStatus_NotFound(t *testing.T) {
	ws := &WorkspaceStatus{
		Files: []FileStatus{
			{Filename: "pixi.toml", Status: StatusClean},
		},
	}

	result := ws.GetFileStatus("nonexistent.txt")
	if result != nil {
		t.Errorf("GetFileStatus(nonexistent) = %v, want nil", result)
	}
}

func TestFileStatusDigests(t *testing.T) {
	dir, nf := setupTestWorkspace(t,
		"original content",
		"original lock",
	)

	// Modify pixi.toml
	newContent := []byte("modified content")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), newContent, 0644)

	ws := CheckWithNebiFile(dir, nf)

	tomlStatus := ws.GetFileStatus("pixi.toml")
	if tomlStatus == nil {
		t.Fatal("pixi.toml status not found")
	}

	// Origin digest should match what .nebi has
	if tomlStatus.OriginDigest != nf.GetLayerDigest("pixi.toml") {
		t.Errorf("OriginDigest = %q, want %q", tomlStatus.OriginDigest, nf.GetLayerDigest("pixi.toml"))
	}

	// Current digest should match the new content
	expectedCurrent := nebifile.ComputeDigest(newContent)
	if tomlStatus.CurrentDigest != expectedCurrent {
		t.Errorf("CurrentDigest = %q, want %q", tomlStatus.CurrentDigest, expectedCurrent)
	}
}

func TestStatusConstants(t *testing.T) {
	if StatusClean != "clean" {
		t.Errorf("StatusClean = %q, want %q", StatusClean, "clean")
	}
	if StatusModified != "modified" {
		t.Errorf("StatusModified = %q, want %q", StatusModified, "modified")
	}
	if StatusMissing != "missing" {
		t.Errorf("StatusMissing = %q, want %q", StatusMissing, "missing")
	}
	if StatusUnknown != "unknown" {
		t.Errorf("StatusUnknown = %q, want %q", StatusUnknown, "unknown")
	}
}

func TestCheck_WhitespaceOnlyChange(t *testing.T) {
	// Byte-level comparison means even whitespace changes are detected
	dir, _ := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\n",
	)

	// Add a trailing space (byte-level change)
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[workspace]\nname = \"test\" \n"), 0644)

	ws, err := Check(dir)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if ws.Overall != StatusModified {
		t.Errorf("Overall = %q, want %q (whitespace change should be detected)", ws.Overall, StatusModified)
	}
}
