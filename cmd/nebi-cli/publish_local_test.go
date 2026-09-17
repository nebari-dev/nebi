package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/zalando/go-keyring"
)

func init() {
	keyring.MockInit()
}

func TestLocalPublishDefaults(t *testing.T) {
	// Set up a temp store
	dataDir := t.TempDir()
	s, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer s.Close()

	// Create workspace directory with pixi files
	wsDir := t.TempDir()
	pixiToml := `[project]\nname = "test-workspace"\nversion = "0.1.0"`
	pixiLock := `version: 6\npackages: []`

	os.WriteFile(filepath.Join(wsDir, "pixi.toml"), []byte(pixiToml), 0644)
	os.WriteFile(filepath.Join(wsDir, "pixi.lock"), []byte(pixiLock), 0644)

	// Create workspace in store
	ws := &store.LocalWorkspace{
		Name: "test-workspace",
		Path: wsDir,
	}
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Verify default tag is content hash
	expectedTag := contenthash.Hash(pixiToml, pixiLock)
	if len(expectedTag) != 16 {
		t.Fatalf("unexpected tag length: %q", expectedTag)
	}

	// Verify default repo name format
	expectedRepo := "test-workspace-" + ws.ID.String()[:8]
	if len(expectedRepo) < len("test-workspace-12345678") {
		t.Fatalf("unexpected repo format: %q", expectedRepo)
	}

	// Create a registry
	reg := &store.LocalRegistry{
		Name:      "test-registry",
		URL:       "ghcr.io",
		Username:  "testuser",
		IsDefault: true,
		Namespace: "testorg",
	}
	if err := s.CreateRegistry(reg); err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}

	// Store credentials
	cs := store.NewCredentialStore(dataDir)
	if err := cs.SetPassword("test-registry", "testpass"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	// Verify we can retrieve everything needed for publish
	defaultReg, err := s.GetDefaultRegistry()
	if err != nil {
		t.Fatalf("GetDefaultRegistry: %v", err)
	}
	if defaultReg.Name != "test-registry" {
		t.Fatalf("expected default registry 'test-registry', got %q", defaultReg.Name)
	}

	pw, err := cs.GetPassword("test-registry")
	if err != nil {
		t.Fatalf("GetPassword: %v", err)
	}
	if pw != "testpass" {
		t.Fatalf("expected password 'testpass', got %q", pw)
	}

	// Verify publication can be recorded
	pub := &store.LocalPublication{
		WorkspaceID: ws.ID,
		RegistryID:  reg.ID,
		Repository:  "ghcr.io/testorg/" + expectedRepo,
		Tag:         expectedTag,
		Digest:      "sha256:fake",
	}
	if err := s.CreatePublication(pub); err != nil {
		t.Fatalf("CreatePublication: %v", err)
	}

	pubs, err := s.ListPublicationsByWorkspace(ws.ID)
	if err != nil {
		t.Fatalf("ListPublicationsByWorkspace: %v", err)
	}
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publication, got %d", len(pubs))
	}
	if pubs[0].Tag != expectedTag {
		t.Fatalf("expected tag %q, got %q", expectedTag, pubs[0].Tag)
	}
}
