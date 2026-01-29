package localstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)

	// Empty index on first load
	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex on empty dir: %v", err)
	}
	if len(idx.Workspaces) != 0 {
		t.Fatalf("expected empty workspaces, got %d", len(idx.Workspaces))
	}

	// Save and reload
	now := time.Now().Truncate(time.Second)
	idx.Workspaces["/home/user/project"] = &Workspace{
		ID:        "abc-123",
		Name:      "project",
		Path:      "/home/user/project",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	idx2, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex after save: %v", err)
	}

	ws, ok := idx2.Workspaces["/home/user/project"]
	if !ok {
		t.Fatal("workspace not found after reload")
	}
	if ws.ID != "abc-123" || ws.Name != "project" {
		t.Fatalf("unexpected workspace: %+v", ws)
	}
}

func TestSaveSnapshotAndExists(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)

	// Create a fake workspace dir with pixi.toml
	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte("[project]\nname = \"test\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if store.SnapshotExists("ws-1") {
		t.Fatal("snapshot should not exist yet")
	}

	if err := store.SaveSnapshot("ws-1", wsDir); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	if !store.SnapshotExists("ws-1") {
		t.Fatal("snapshot should exist after save")
	}

	// Verify pixi.toml was copied
	data, err := os.ReadFile(filepath.Join(store.SnapshotDir("ws-1"), "pixi.toml"))
	if err != nil {
		t.Fatalf("reading snapshot: %v", err)
	}
	if string(data) != "[project]\nname = \"test\"\n" {
		t.Fatalf("unexpected snapshot content: %q", data)
	}
}

func TestComputeStatus_Clean(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)

	wsDir := t.TempDir()
	content := []byte("[project]\nname = \"test\"\n")
	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), content, 0644)

	ws := &Workspace{ID: "ws-1", Path: wsDir}
	store.SaveSnapshot("ws-1", wsDir)

	status := store.ComputeStatus(ws)
	if status != StatusClean {
		t.Fatalf("expected clean, got %s", status)
	}
}

func TestComputeStatus_Modified(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)

	wsDir := t.TempDir()
	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte("original"), 0644)

	ws := &Workspace{ID: "ws-2", Path: wsDir}
	store.SaveSnapshot("ws-2", wsDir)

	// Modify the file
	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte("modified"), 0644)

	status := store.ComputeStatus(ws)
	if status != StatusModified {
		t.Fatalf("expected modified, got %s", status)
	}
}

func TestComputeStatus_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)

	ws := &Workspace{ID: "ws-3", Path: "/nonexistent/path"}

	status := store.ComputeStatus(ws)
	if status != StatusMissing {
		t.Fatalf("expected missing, got %s", status)
	}
}
