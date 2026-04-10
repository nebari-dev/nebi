package store

import (
	"os"
	"path/filepath"
	"testing"
)

// makeWorkspaceWithDir creates a tracked workspace pointing at a fresh
// temp directory and returns it.
func makeWorkspaceWithDir(t *testing.T, s *Store, name string) *LocalWorkspace {
	t.Helper()
	dir := t.TempDir()
	ws := &LocalWorkspace{Name: name, Path: dir}
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return ws
}

func TestCreateVersion_DedupAgainstLatest(t *testing.T) {
	s := testStore(t)
	ws := makeWorkspaceWithDir(t, s, "dedup-test")

	manifest := "[workspace]\nname = \"dedup-test\"\n"
	lock := "version: 6\n"

	v1, created, err := s.CreateVersion(ws.ID, manifest, lock, "first")
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	if !created {
		t.Fatal("first version should report created=true")
	}
	if v1.VersionNumber != 1 {
		t.Fatalf("expected version 1, got %d", v1.VersionNumber)
	}

	// Same content → dedup → should reuse v1.
	v2, created, err := s.CreateVersion(ws.ID, manifest, lock, "second attempt")
	if err != nil {
		t.Fatalf("CreateVersion (dedup): %v", err)
	}
	if created {
		t.Fatal("identical content should report created=false")
	}
	if v2.VersionNumber != 1 {
		t.Fatalf("expected dedup to return version 1, got %d", v2.VersionNumber)
	}

	// Different content → new version.
	v3, created, err := s.CreateVersion(ws.ID, manifest+"\n# tweak\n", lock, "third")
	if err != nil {
		t.Fatalf("CreateVersion (new): %v", err)
	}
	if !created {
		t.Fatal("changed content should report created=true")
	}
	if v3.VersionNumber != 2 {
		t.Fatalf("expected version 2, got %d", v3.VersionNumber)
	}
}

func TestListVersions(t *testing.T) {
	s := testStore(t)
	ws := makeWorkspaceWithDir(t, s, "list-test")

	// Empty
	versions, err := s.ListVersions(ws.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions, got %d", len(versions))
	}

	// Three distinct versions
	for i := 1; i <= 3; i++ {
		manifest := "[workspace]\nv = " + string(rune('0'+i)) + "\n"
		if _, _, err := s.CreateVersion(ws.ID, manifest, "", "v"+string(rune('0'+i))); err != nil {
			t.Fatalf("CreateVersion %d: %v", i, err)
		}
	}

	versions, err = s.ListVersions(ws.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// Newest first
	if versions[0].VersionNumber != 3 || versions[2].VersionNumber != 1 {
		t.Fatalf("expected version_number DESC ordering, got %d/%d/%d",
			versions[0].VersionNumber, versions[1].VersionNumber, versions[2].VersionNumber)
	}
	// ListVersions should NOT load the heavy text columns.
	if versions[0].ManifestContent != "" {
		t.Fatalf("expected ManifestContent to be empty in list, got %q", versions[0].ManifestContent)
	}
}

func TestGetVersion(t *testing.T) {
	s := testStore(t)
	ws := makeWorkspaceWithDir(t, s, "get-test")

	manifest := "[workspace]\nname = \"get-test\"\n"
	lock := "version: 6\nfoo: bar\n"
	if _, _, err := s.CreateVersion(ws.ID, manifest, lock, "first"); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetVersion(ws.ID, 1)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if got == nil {
		t.Fatal("expected version 1 to exist")
	}
	if got.ManifestContent != manifest {
		t.Fatalf("manifest mismatch:\nwant: %q\ngot:  %q", manifest, got.ManifestContent)
	}
	if got.LockFileContent != lock {
		t.Fatalf("lock mismatch:\nwant: %q\ngot:  %q", lock, got.LockFileContent)
	}

	// Missing version → nil, no error
	missing, err := s.GetVersion(ws.ID, 99)
	if err != nil {
		t.Fatalf("GetVersion(missing): %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing version, got %+v", missing)
	}
}

func TestRollbackToVersion(t *testing.T) {
	s := testStore(t)
	ws := makeWorkspaceWithDir(t, s, "rollback-test")

	v1Manifest := "[workspace]\nname = \"rollback-test\"\nversion = \"1\"\n"
	v1Lock := "version: 6\n# v1\n"
	if _, _, err := s.CreateVersion(ws.ID, v1Manifest, v1Lock, "v1"); err != nil {
		t.Fatal(err)
	}

	v2Manifest := "[workspace]\nname = \"rollback-test\"\nversion = \"2\"\n"
	v2Lock := "version: 6\n# v2\n"
	if _, _, err := s.CreateVersion(ws.ID, v2Manifest, v2Lock, "v2"); err != nil {
		t.Fatal(err)
	}

	// Simulate a "current" disk state matching v2.
	if err := os.WriteFile(filepath.Join(ws.Path, "pixi.toml"), []byte(v2Manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "pixi.lock"), []byte(v2Lock), 0644); err != nil {
		t.Fatal(err)
	}

	// Roll back to v1.
	v3, err := s.RollbackToVersion(ws.ID, 1)
	if err != nil {
		t.Fatalf("RollbackToVersion: %v", err)
	}
	if v3.VersionNumber != 3 {
		t.Fatalf("expected new snapshot to be version 3, got %d", v3.VersionNumber)
	}
	if v3.Description != "Rolled back to version 1" {
		t.Fatalf("unexpected description: %q", v3.Description)
	}

	// Disk should now match v1 again.
	gotManifest, err := os.ReadFile(filepath.Join(ws.Path, "pixi.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotManifest) != v1Manifest {
		t.Fatalf("pixi.toml not restored:\nwant: %q\ngot:  %q", v1Manifest, gotManifest)
	}
	gotLock, err := os.ReadFile(filepath.Join(ws.Path, "pixi.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotLock) != v1Lock {
		t.Fatalf("pixi.lock not restored:\nwant: %q\ngot:  %q", v1Lock, gotLock)
	}

	// And the new snapshot should hold the rolled-back content.
	stored, err := s.GetVersion(ws.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ManifestContent != v1Manifest {
		t.Fatalf("v3 manifest content unexpected: %q", stored.ManifestContent)
	}
}

func TestRollbackToVersion_NotFound(t *testing.T) {
	s := testStore(t)
	ws := makeWorkspaceWithDir(t, s, "rb-missing")

	if _, err := s.RollbackToVersion(ws.ID, 42); err == nil {
		t.Fatal("expected error rolling back to nonexistent version")
	}
}
