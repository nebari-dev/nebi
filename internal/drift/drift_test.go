package drift

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
)

func setupTestWorkspace(t *testing.T, pixiToml, pixiLock string) (string, *nebifile.NebiFile, map[string]string) {
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

	// Create layers map for drift detection
	layers := map[string]string{
		"pixi.toml": tomlDigest,
		"pixi.lock": lockDigest,
	}

	// Create .nebi file
	nf := nebifile.NewFromPull(
		"test-workspace", "v1.0", "https://nebi.example.com", "", "1", "",
	)
	nebifile.Write(dir, nf)

	return dir, nf, layers
}

func TestCheck_Clean(t *testing.T) {
	dir, _, layers := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	ws := CheckWithLayers(dir, layers)

	if ws.Overall != StatusClean {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusClean)
	}
	if ws.IsModified() {
		t.Error("IsModified() should be false for clean workspace")
	}
}

func TestCheck_ModifiedToml(t *testing.T) {
	dir, _, layers := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	// Modify pixi.toml
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("modified content"), 0644)

	ws := CheckWithLayers(dir, layers)

	if ws.Overall != StatusModified {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusModified)
	}

	tomlStatus := ws.GetFileStatus("pixi.toml")
	if tomlStatus == nil {
		t.Fatal("Expected pixi.toml status")
	}
	if tomlStatus.Status != StatusModified {
		t.Errorf("pixi.toml status = %q, want %q", tomlStatus.Status, StatusModified)
	}
}

func TestCheck_MissingFile(t *testing.T) {
	dir, _, layers := setupTestWorkspace(t,
		"[workspace]\nname = \"test\"\n",
		"version: 1\npackages: []\n",
	)

	// Delete pixi.lock
	os.Remove(filepath.Join(dir, "pixi.lock"))

	ws := CheckWithLayers(dir, layers)

	if ws.Overall != StatusModified {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusModified)
	}

	lockStatus := ws.GetFileStatus("pixi.lock")
	if lockStatus == nil {
		t.Fatal("Expected pixi.lock status")
	}
	if lockStatus.Status != StatusMissing {
		t.Errorf("pixi.lock status = %q, want %q", lockStatus.Status, StatusMissing)
	}
}

func TestCheckWithLayers(t *testing.T) {
	dir := t.TempDir()

	content := []byte("test content")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), content, 0644)

	digest := nebifile.ComputeDigest(content)
	layers := map[string]string{
		"pixi.toml": digest,
	}

	ws := CheckWithLayers(dir, layers)
	if ws.Overall != StatusClean {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusClean)
	}
}

func TestGetFileStatus_NotFound(t *testing.T) {
	ws := &RepoStatus{
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
	dir, _, layers := setupTestWorkspace(t,
		"original content",
		"original lock",
	)

	// Modify pixi.toml
	newContent := []byte("modified content")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), newContent, 0644)

	ws := CheckWithLayers(dir, layers)

	tomlStatus := ws.GetFileStatus("pixi.toml")
	if tomlStatus == nil {
		t.Fatal("pixi.toml status not found")
	}

	// Origin digest should match what layers have
	if tomlStatus.OriginDigest != layers["pixi.toml"] {
		t.Errorf("OriginDigest = %q, want %q", tomlStatus.OriginDigest, layers["pixi.toml"])
	}

	// Current digest should match the new content
	expectedCurrent := nebifile.ComputeDigest(newContent)
	if tomlStatus.CurrentDigest != expectedCurrent {
		t.Errorf("CurrentDigest = %q, want %q", tomlStatus.CurrentDigest, expectedCurrent)
	}
}

func TestCheckWithNebiFileIntegration(t *testing.T) {
	// This tests the full integration with localindex
	dir := t.TempDir()
	indexDir := t.TempDir()

	// Create local files
	tomlContent := []byte("[workspace]\nname = \"test\"\n")
	lockContent := []byte("version: 1\npackages: []\n")
	os.WriteFile(filepath.Join(dir, "pixi.toml"), tomlContent, 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), lockContent, 0644)

	tomlDigest := nebifile.ComputeDigest(tomlContent)
	lockDigest := nebifile.ComputeDigest(lockContent)

	// Create nebifile
	nf := nebifile.NewFromPull(
		"test-repo", "v1.0", "https://nebi.example.com", "", "1", "",
	)
	nebifile.Write(dir, nf)

	// Create index entry with layers
	store := localindex.NewStoreWithDir(indexDir)
	store.AddEntry(localindex.Entry{
		SpecName:    "test-repo",
		VersionName: "v1.0",
		VersionID:   "1",
		Path:        dir,
		PulledAt:    time.Now(),
		Layers: map[string]string{
			"pixi.toml": tomlDigest,
			"pixi.lock": lockDigest,
		},
	})

	// Use CheckWithLayers directly with the entry's layers
	entry, _ := store.FindByPath(dir)
	ws := CheckWithLayers(dir, entry.Layers)

	if ws.Overall != StatusClean {
		t.Errorf("Overall = %q, want %q", ws.Overall, StatusClean)
	}
}
