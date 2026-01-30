package localstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeDiff_Clean(t *testing.T) {
	storeDir := t.TempDir()
	store := NewStoreWithDir(storeDir)

	wsDir := t.TempDir()
	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte("content"), 0644)

	ws := &Workspace{ID: "ws-1", Path: wsDir}
	store.SaveSnapshot("ws-1", wsDir)

	results, err := store.ComputeDiff(ws)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range results {
		if r.Status != FileUnchanged && r.Status != FileBothMissing {
			t.Fatalf("expected unchanged or both missing, got %d for %s", r.Status, r.Name)
		}
	}
}

func TestComputeDiff_Modified(t *testing.T) {
	storeDir := t.TempDir()
	store := NewStoreWithDir(storeDir)

	wsDir := t.TempDir()
	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte("original"), 0644)

	ws := &Workspace{ID: "ws-2", Path: wsDir}
	store.SaveSnapshot("ws-2", wsDir)

	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte("modified"), 0644)

	results, err := store.ComputeDiff(ws)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range results {
		if r.Name == "pixi.toml" {
			if r.Status != FileModified {
				t.Fatalf("expected modified, got %d", r.Status)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("pixi.toml not in results")
	}
}

func TestComputeDiff_NewFile(t *testing.T) {
	storeDir := t.TempDir()
	store := NewStoreWithDir(storeDir)

	wsDir := t.TempDir()
	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte("content"), 0644)

	ws := &Workspace{ID: "ws-3", Path: wsDir}
	// No snapshot saved — simulate new file

	// Create snapshot dir but only with pixi.toml missing
	os.MkdirAll(store.SnapshotDir("ws-3"), 0755)

	results, err := store.ComputeDiff(ws)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range results {
		if r.Name == "pixi.toml" && r.Status != FileNew {
			t.Fatalf("expected new, got %d", r.Status)
		}
	}
}
