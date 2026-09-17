package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSizeUpdatePreservesRenamedWorkspace(t *testing.T) {
	svc, db := testSetup(t, true)
	owner := createTestUser(t, db, "owner")
	ws := createReadyWorkspace(t, svc, db, "before", owner)
	ws.Path = t.TempDir()
	if err := os.WriteFile(filepath.Join(ws.Path, "data"), []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(ws).Updates(map[string]interface{}{"name": "after", "path": ws.Path}).Error; err != nil {
		t.Fatal(err)
	}
	ws.Name = "before" // object loaded before a concurrent metadata rename
	svc.UpdateWorkspaceSize(ws)
	got, err := svc.Get(ws.ID.String())
	if err != nil || got.Name != "after" || got.SizeBytes != 7 {
		t.Fatalf("size update must preserve the renamed label: %+v %v", got, err)
	}
}
