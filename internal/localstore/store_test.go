package localstore

import (
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
