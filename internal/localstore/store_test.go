package localstore

import (
	"path/filepath"
	"testing"
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
	idx.Workspaces["/home/user/project"] = &Workspace{
		ID:   "abc-123",
		Name: "project",
		Path: "/home/user/project",
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

func TestGlobalWorkspaceRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)

	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}

	// Add a global workspace
	id := "test-uuid-123"
	envDir := store.GlobalEnvDir(id)

	idx.Workspaces[envDir] = &Workspace{
		ID:     id,
		Name:   "data-science",
		Path:   envDir,
		Global: true,
	}

	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	// Reload and verify
	idx2, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex after save: %v", err)
	}

	ws, ok := idx2.Workspaces[envDir]
	if !ok {
		t.Fatal("global workspace not found after reload")
	}
	if ws.Name != "data-science" {
		t.Errorf("expected name 'data-science', got %q", ws.Name)
	}
	if !ws.Global {
		t.Error("expected Global to be true")
	}
	if ws.Path != envDir {
		t.Errorf("expected path %q, got %q", envDir, ws.Path)
	}
}

func TestGlobalEnvDir(t *testing.T) {
	store := NewStoreWithDir("/tmp/nebi-test")
	dir := store.GlobalEnvDir("abc-123")
	expected := filepath.Join("/tmp/nebi-test", "environments", "abc-123")
	if dir != expected {
		t.Errorf("GlobalEnvDir: got %q, want %q", dir, expected)
	}
}

func TestGlobalFieldOmittedWhenFalse(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)

	idx, _ := store.LoadIndex()
	idx.Workspaces["/home/user/project"] = &Workspace{
		ID:   "local-123",
		Name: "project",
		Path: "/home/user/project",
	}

	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	// Reload and verify Global defaults to false
	idx2, _ := store.LoadIndex()
	ws := idx2.Workspaces["/home/user/project"]
	if ws.Global {
		t.Error("expected Global to be false for local workspace")
	}
}
