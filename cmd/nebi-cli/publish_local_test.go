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

	// Create project directory with pixi files
	projectDir := t.TempDir()
	pixiToml := `[project]\nname = "test-project"\nversion = "0.1.0"`
	pixiLock := `version: 6\npackages: []`

	os.WriteFile(filepath.Join(projectDir, "pixi.toml"), []byte(pixiToml), 0644)
	os.WriteFile(filepath.Join(projectDir, "pixi.lock"), []byte(pixiLock), 0644)

	// Create project in store
	project := &store.LocalProject{
		Name: "test-project",
		Path: projectDir,
	}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Verify default tag is content hash
	expectedTag := contenthash.Hash(pixiToml, pixiLock)
	if len(expectedTag) != 16 {
		t.Fatalf("unexpected tag length: %q", expectedTag)
	}

	// Verify default repo name format
	expectedRepo := "test-project-" + project.ID.String()[:8]
	if len(expectedRepo) < len("test-project-12345678") {
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
		ProjectID:  project.ID,
		RegistryID: reg.ID,
		Repository: "ghcr.io/testorg/" + expectedRepo,
		Tag:        expectedTag,
		Digest:     "sha256:fake",
	}
	if err := s.CreatePublication(pub); err != nil {
		t.Fatalf("CreatePublication: %v", err)
	}

	pubs, err := s.ListPublicationsByProject(project.ID)
	if err != nil {
		t.Fatalf("ListPublicationsByProject: %v", err)
	}
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publication, got %d", len(pubs))
	}
	if pubs[0].Tag != expectedTag {
		t.Fatalf("expected tag %q, got %q", expectedTag, pubs[0].Tag)
	}
}
