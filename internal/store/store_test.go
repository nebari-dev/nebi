package store

import (
	"testing"

	"github.com/google/uuid"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestWorkspaceRoundTrip(t *testing.T) {
	s := testStore(t)

	// Empty initially
	wss, err := s.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected 0 workspaces, got %d", len(wss))
	}

	// Create
	ws := &LocalWorkspace{
		Name: "project",
		Path: "/home/user/project",
	}
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if ws.ID == uuid.Nil {
		t.Fatal("expected non-nil ID after create")
	}

	// List
	wss, _ = s.ListWorkspaces()
	if len(wss) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(wss))
	}
	if wss[0].Name != "project" {
		t.Fatalf("expected name 'project', got %q", wss[0].Name)
	}

	// Get by ID
	got, err := s.GetWorkspace(ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if got.Path != "/home/user/project" {
		t.Fatalf("unexpected path: %q", got.Path)
	}

	// Find by path
	found, err := s.FindWorkspaceByPath("/home/user/project")
	if err != nil {
		t.Fatalf("FindWorkspaceByPath: %v", err)
	}
	if found == nil || found.Name != "project" {
		t.Fatal("expected to find workspace by path")
	}

	// Find by path - not found
	notFound, err := s.FindWorkspaceByPath("/nonexistent")
	if err != nil {
		t.Fatalf("FindWorkspaceByPath: %v", err)
	}
	if notFound != nil {
		t.Fatal("expected nil for nonexistent path")
	}

	// Delete
	if err := s.DeleteWorkspace(ws.ID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	wss, _ = s.ListWorkspaces()
	if len(wss) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(wss))
	}
}

func TestFindWorkspaceByName(t *testing.T) {
	s := testStore(t)

	ws := &LocalWorkspace{
		Name: "data-science",
		Path: "/home/user/data-science",
	}
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	found, err := s.FindWorkspaceByName("data-science")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil {
		t.Fatal("expected to find workspace by name")
	}
	if found.Name != "data-science" {
		t.Errorf("expected name 'data-science', got %q", found.Name)
	}

	// Not found
	notFound, err := s.FindWorkspaceByName("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if notFound != nil {
		t.Fatal("expected nil for nonexistent name")
	}
}

func TestOriginFields(t *testing.T) {
	s := testStore(t)

	ws := &LocalWorkspace{
		Name:           "project",
		Path:           "/home/user/project",
		OriginName:     "my-env",
		OriginTag:      "v1.0",
		OriginAction:   "push",
		OriginTomlHash: "abc123",
		OriginLockHash: "def456",
	}
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetWorkspace(ws.ID)
	if got.OriginName != "my-env" || got.OriginTag != "v1.0" || got.OriginAction != "push" {
		t.Fatalf("unexpected origin: name=%q tag=%q action=%q", got.OriginName, got.OriginTag, got.OriginAction)
	}
	if got.OriginTomlHash != "abc123" || got.OriginLockHash != "def456" {
		t.Fatalf("unexpected hashes: toml=%q lock=%q", got.OriginTomlHash, got.OriginLockHash)
	}
}

func TestCredentialsRoundTrip(t *testing.T) {
	s := testStore(t)

	// Empty initially
	creds, err := s.LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if creds.Token != "" || creds.Username != "" {
		t.Fatalf("expected empty creds, got %+v", creds)
	}

	// Save
	if err := s.SaveCredentials(&Credentials{Token: "test-token", Username: "admin"}); err != nil {
		t.Fatal(err)
	}

	// Load
	creds, _ = s.LoadCredentials()
	if creds.Token != "test-token" || creds.Username != "admin" {
		t.Fatalf("unexpected creds: %+v", creds)
	}

	// Overwrite
	s.SaveCredentials(&Credentials{Token: "new-token", Username: "user2"})
	creds, _ = s.LoadCredentials()
	if creds.Token != "new-token" || creds.Username != "user2" {
		t.Fatalf("unexpected creds after overwrite: %+v", creds)
	}
}

func TestServerURL(t *testing.T) {
	s := testStore(t)

	url, _ := s.LoadServerURL()
	if url != "" {
		t.Fatalf("expected empty URL, got %q", url)
	}

	s.SaveServerURL("https://nebi.example.com")
	url, _ = s.LoadServerURL()
	if url != "https://nebi.example.com" {
		t.Fatalf("expected URL, got %q", url)
	}
}

func TestDefaults(t *testing.T) {
	s := testStore(t)

	ws := &LocalWorkspace{
		Name: "test",
		Path: "/tmp/test",
	}
	s.CreateWorkspace(ws)

	got, _ := s.GetWorkspace(ws.ID)
	if got.Status != "ready" {
		t.Errorf("expected status 'ready', got %q", got.Status)
	}
	if got.Source != "local" {
		t.Errorf("expected source 'local', got %q", got.Source)
	}
	if got.PackageManager != "pixi" {
		t.Errorf("expected package_manager 'pixi', got %q", got.PackageManager)
	}
}
