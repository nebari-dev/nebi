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

func TestFindWorkspacesByName(t *testing.T) {
	s := testStore(t)

	// Create two workspaces with the same name but different paths
	ws1 := &LocalWorkspace{Name: "data-science", Path: "/home/user/project-a"}
	ws2 := &LocalWorkspace{Name: "data-science", Path: "/home/user/project-b"}
	ws3 := &LocalWorkspace{Name: "other", Path: "/home/user/other"}
	for _, ws := range []*LocalWorkspace{ws1, ws2, ws3} {
		if err := s.CreateWorkspace(ws); err != nil {
			t.Fatal(err)
		}
	}

	// Should return both data-science workspaces
	found, err := s.FindWorkspacesByName("data-science")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(found))
	}

	// Should return one for "other"
	found, err = s.FindWorkspacesByName("other")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(found))
	}

	// Should return empty for nonexistent
	found, err = s.FindWorkspacesByName("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("expected 0 workspaces, got %d", len(found))
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

func TestRegistryCRUD(t *testing.T) {
	s := testStore(t)

	// Empty initially
	regs, err := s.ListRegistries()
	if err != nil {
		t.Fatalf("ListRegistries: %v", err)
	}
	if len(regs) != 0 {
		t.Fatalf("expected 0 registries, got %d", len(regs))
	}

	// Create
	reg := &LocalRegistry{
		Name:      "ghcr",
		URL:       "ghcr.io",
		Username:  "myuser",
		IsDefault: true,
		Namespace: "myorg",
	}
	if err := s.CreateRegistry(reg); err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}
	if reg.ID == uuid.Nil {
		t.Fatal("expected non-nil ID after create")
	}

	// List
	regs, _ = s.ListRegistries()
	if len(regs) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(regs))
	}
	if regs[0].Name != "ghcr" {
		t.Fatalf("expected name 'ghcr', got %q", regs[0].Name)
	}

	// Get by name
	got, err := s.GetRegistryByName("ghcr")
	if err != nil {
		t.Fatalf("GetRegistryByName: %v", err)
	}
	if got.URL != "ghcr.io" {
		t.Fatalf("expected URL 'ghcr.io', got %q", got.URL)
	}

	// Get by name - not found
	_, err = s.GetRegistryByName("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent registry")
	}

	// Get default
	def, err := s.GetDefaultRegistry()
	if err != nil {
		t.Fatalf("GetDefaultRegistry: %v", err)
	}
	if def.Name != "ghcr" {
		t.Fatalf("expected default to be 'ghcr', got %q", def.Name)
	}

	// Delete
	if err := s.DeleteRegistry(reg.ID); err != nil {
		t.Fatalf("DeleteRegistry: %v", err)
	}
	regs, _ = s.ListRegistries()
	if len(regs) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(regs))
	}
}

func TestGetDefaultRegistry_NoneSet(t *testing.T) {
	s := testStore(t)

	_, err := s.GetDefaultRegistry()
	if err == nil {
		t.Fatal("expected error when no default registry set")
	}
}

func TestRegistryUniqueName(t *testing.T) {
	s := testStore(t)

	reg1 := &LocalRegistry{Name: "ghcr", URL: "ghcr.io"}
	if err := s.CreateRegistry(reg1); err != nil {
		t.Fatal(err)
	}

	reg2 := &LocalRegistry{Name: "ghcr", URL: "other.io"}
	if err := s.CreateRegistry(reg2); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestPublicationCRUD(t *testing.T) {
	s := testStore(t)

	// Create a workspace and registry first (foreign keys)
	ws := &LocalWorkspace{Name: "test-ws", Path: "/tmp/test"}
	s.CreateWorkspace(ws)

	reg := &LocalRegistry{Name: "ghcr", URL: "ghcr.io", IsDefault: true}
	s.CreateRegistry(reg)

	// Empty initially
	pubs, err := s.ListPublications()
	if err != nil {
		t.Fatalf("ListPublications: %v", err)
	}
	if len(pubs) != 0 {
		t.Fatalf("expected 0 publications, got %d", len(pubs))
	}

	// Create
	pub := &LocalPublication{
		WorkspaceID: ws.ID,
		RegistryID:  reg.ID,
		Repository:  "ghcr.io/myorg/test-ws-12345678",
		Tag:         "sha-abcdef123456",
		Digest:      "sha256:deadbeef",
	}
	if err := s.CreatePublication(pub); err != nil {
		t.Fatalf("CreatePublication: %v", err)
	}
	if pub.ID == uuid.Nil {
		t.Fatal("expected non-nil ID after create")
	}

	// List all
	pubs, _ = s.ListPublications()
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publication, got %d", len(pubs))
	}
	if pubs[0].Repository != "ghcr.io/myorg/test-ws-12345678" {
		t.Fatalf("unexpected repository: %q", pubs[0].Repository)
	}

	// List by workspace
	pubs, err = s.ListPublicationsByWorkspace(ws.ID)
	if err != nil {
		t.Fatalf("ListPublicationsByWorkspace: %v", err)
	}
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publication for workspace, got %d", len(pubs))
	}

	// List by workspace - different ID returns empty
	pubs, _ = s.ListPublicationsByWorkspace(uuid.New())
	if len(pubs) != 0 {
		t.Fatalf("expected 0 publications for other workspace, got %d", len(pubs))
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
