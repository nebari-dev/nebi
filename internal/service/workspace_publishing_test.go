package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

// --- GetPublishDefaults tests ---

func TestGetPublishDefaults_ReturnsDefaults(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "publish-test", userID)

	// Migrate OCIRegistry and create a default registry
	// OCIRegistry already migrated in testSetup
	registry := models.OCIRegistry{
		Name:      "test-registry",
		URL:       "https://quay.io",
		Namespace: "myorg",
		IsDefault: true,
	}
	db.Create(&registry)

	defaults, err := svc.GetPublishDefaults(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if defaults.RegistryID != registry.ID {
		t.Errorf("expected registry ID %s, got %s", registry.ID, defaults.RegistryID)
	}
	if defaults.RegistryName != "test-registry" {
		t.Errorf("expected registry name %q, got %q", "test-registry", defaults.RegistryName)
	}
	if defaults.Namespace != "myorg" {
		t.Errorf("expected namespace %q, got %q", "myorg", defaults.Namespace)
	}

	// Repo should be name-first8charsOfID
	expectedRepo := "publish-test-" + ws.ID.String()[:8]
	if defaults.Repository != expectedRepo {
		t.Errorf("expected repository %q, got %q", expectedRepo, defaults.Repository)
	}

	// No versions pushed, so tag should default to "latest"
	if defaults.Tag != "latest" {
		t.Errorf("expected tag %q, got %q", "latest", defaults.Tag)
	}
}

func TestGetPublishDefaults_UsesContentHashTag(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "publish-hash", userID)

	// OCIRegistry already migrated in testSetup
	db.Create(&models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", IsDefault: true})

	// Push a version so there's a content hash
	svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		PixiToml: "[project]\nname = \"test\"",
		PixiLock: "version: 6",
	}, userID)

	defaults, err := svc.GetPublishDefaults(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tag should be the content hash, not "latest"
	if defaults.Tag == "latest" {
		t.Error("expected content hash tag, got 'latest'")
	}
	if len(defaults.Tag) < 10 {
		t.Errorf("expected sha-prefixed hash tag, got %q", defaults.Tag)
	}
}

func TestGetPublishDefaults_NoDefaultRegistry(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "no-reg", userID)

	// OCIRegistry already migrated in testSetup
	// No default registry exists

	_, err := svc.GetPublishDefaults(ws.ID.String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPublishDefaults_WorkspaceNotFound(t *testing.T) {
	svc, _ := testSetup(t, false)

	_, err := svc.GetPublishDefaults(uuid.New().String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListPublications tests ---

func TestListPublications_Empty(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "no-pubs", userID)

	// OCIRegistry and Publication already migrated in testSetup

	pubs, err := svc.ListPublications(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pubs) != 0 {
		t.Errorf("expected 0 publications, got %d", len(pubs))
	}
}

func TestListPublications_ReturnsRecords(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "with-pubs", userID)

	// OCIRegistry and Publication already migrated in testSetup

	registry := models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", Namespace: "myorg"}
	db.Create(&registry)

	pub := models.Publication{
		WorkspaceID:   ws.ID,
		VersionNumber: 1,
		RegistryID:    registry.ID,
		Repository:    "my-env",
		Tag:           "v1.0.0",
		Digest:        "sha256:abc123",
		PublishedBy:   userID,
	}
	db.Create(&pub)

	pubs, err := svc.ListPublications(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publication, got %d", len(pubs))
	}

	if pubs[0].Repository != "my-env" {
		t.Errorf("expected repository %q, got %q", "my-env", pubs[0].Repository)
	}
	if pubs[0].Tag != "v1.0.0" {
		t.Errorf("expected tag %q, got %q", "v1.0.0", pubs[0].Tag)
	}
	if pubs[0].RegistryName != "reg" {
		t.Errorf("expected registry name %q, got %q", "reg", pubs[0].RegistryName)
	}
}

func TestListPublications_NotFound(t *testing.T) {
	svc, _ := testSetup(t, false)

	_, err := svc.ListPublications(uuid.New().String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- UpdatePublication tests ---

func TestUpdatePublication_TogglesVisibility(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "toggle-vis", userID)

	// OCIRegistry and Publication already migrated in testSetup

	registry := models.OCIRegistry{Name: "reg", URL: "https://ghcr.io"}
	db.Create(&registry)

	pub := models.Publication{
		WorkspaceID:   ws.ID,
		VersionNumber: 1,
		RegistryID:    registry.ID,
		Repository:    "my-env",
		Tag:           "v1",
		PublishedBy:   userID,
		IsPublic:      false,
	}
	db.Create(&pub)

	result, err := svc.UpdatePublication(context.Background(), ws.ID.String(), pub.ID.String(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsPublic {
		t.Error("expected IsPublic=true after update")
	}

	// Verify in DB
	var updated models.Publication
	db.First(&updated, pub.ID)
	if !updated.IsPublic {
		t.Error("expected IsPublic=true in DB")
	}
}

func TestUpdatePublication_NotFound(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "pub-nf", userID)

	// OCIRegistry and Publication already migrated in testSetup

	_, err := svc.UpdatePublication(context.Background(), ws.ID.String(), uuid.New().String(), true)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
