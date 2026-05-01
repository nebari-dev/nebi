package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

func registryTestSetup(t *testing.T) (*RegistryService, *gorm.DB) {
	t.Helper()
	_, db := testSetup(t, false)
	// Use a test encryption key
	encKey := []byte("test-encryption-key-32bytes!!!!!")
	return NewRegistryService(db, encKey), db
}

// --- CreateRegistry ---

func TestRegistryCreate(t *testing.T) {
	svc, _ := registryTestSetup(t)

	result, err := svc.CreateRegistry(CreateRegistryReq{
		Name:     "test-registry",
		URL:      "https://ghcr.io",
		Username: "user",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test-registry" {
		t.Errorf("expected name 'test-registry', got %q", result.Name)
	}
	if result.Username != "user" {
		t.Errorf("expected username 'user', got %q", result.Username)
	}
}

func TestRegistryCreate_DuplicateName(t *testing.T) {
	svc, _ := registryTestSetup(t)

	svc.CreateRegistry(CreateRegistryReq{Name: "dup", URL: "https://a.io"})

	_, err := svc.CreateRegistry(CreateRegistryReq{Name: "dup", URL: "https://b.io"})
	if err == nil {
		t.Fatal("expected conflict error for duplicate name")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestRegistryCreate_SetsDefault(t *testing.T) {
	svc, db := registryTestSetup(t)

	svc.CreateRegistry(CreateRegistryReq{Name: "r1", URL: "https://a.io", IsDefault: true})
	svc.CreateRegistry(CreateRegistryReq{Name: "r2", URL: "https://b.io", IsDefault: true})

	// Only r2 should be default
	registries, _ := svc.ListRegistries()
	for _, r := range registries {
		if r.Name == "r1" && r.IsDefault {
			t.Error("r1 should not be default after r2 was set as default")
		}
		if r.Name == "r2" && !r.IsDefault {
			t.Error("r2 should be default")
		}
	}
	_ = db // used in setup
}

// --- ListRegistries ---

func TestRegistryList(t *testing.T) {
	svc, _ := registryTestSetup(t)

	svc.CreateRegistry(CreateRegistryReq{Name: "r1", URL: "https://a.io"})
	svc.CreateRegistry(CreateRegistryReq{Name: "r2", URL: "https://b.io"})

	registries, err := svc.ListRegistries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(registries) != 2 {
		t.Errorf("expected 2 registries, got %d", len(registries))
	}
}

// --- ListPublicRegistries ---

func TestRegistryListPublic_HidesCredentials(t *testing.T) {
	svc, _ := registryTestSetup(t)

	svc.CreateRegistry(CreateRegistryReq{
		Name:     "public-reg",
		URL:      "https://ghcr.io",
		Username: "secret-user",
		Password: "secret-pass",
		APIToken: "secret-token",
	})

	registries, err := svc.ListPublicRegistries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(registries) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(registries))
	}

	if registries[0].Username != "" {
		t.Errorf("expected empty username in public listing, got %q", registries[0].Username)
	}
	if registries[0].HasAPIToken {
		t.Error("expected HasAPIToken=false in public listing")
	}
}

// --- GetRegistry ---

func TestRegistryGet(t *testing.T) {
	svc, _ := registryTestSetup(t)

	created, _ := svc.CreateRegistry(CreateRegistryReq{Name: "get-me", URL: "https://a.io"})

	result, err := svc.GetRegistry(created.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "get-me" {
		t.Errorf("expected name 'get-me', got %q", result.Name)
	}
}

func TestRegistryGet_NotFound(t *testing.T) {
	svc, _ := registryTestSetup(t)

	_, err := svc.GetRegistry("nonexistent-id")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- UpdateRegistry ---

func TestRegistryUpdate(t *testing.T) {
	svc, _ := registryTestSetup(t)

	created, _ := svc.CreateRegistry(CreateRegistryReq{Name: "update-me", URL: "https://old.io"})

	newName := "updated-name"
	newURL := "https://new.io"
	result, err := svc.UpdateRegistry(created.ID.String(), UpdateRegistryReq{
		Name: &newName,
		URL:  &newURL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got %q", result.Name)
	}
	if result.URL != "https://new.io" {
		t.Errorf("expected URL 'https://new.io', got %q", result.URL)
	}
}

func TestRegistryUpdate_NotFound(t *testing.T) {
	svc, _ := registryTestSetup(t)

	newName := "nope"
	_, err := svc.UpdateRegistry("nonexistent", UpdateRegistryReq{Name: &newName})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- DeleteRegistry ---

func TestRegistryDelete(t *testing.T) {
	svc, _ := registryTestSetup(t)

	created, _ := svc.CreateRegistry(CreateRegistryReq{Name: "del-me", URL: "https://a.io"})

	if err := svc.DeleteRegistry(created.ID.String()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := svc.GetRegistry(created.ID.String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRegistryDelete_NotFound(t *testing.T) {
	svc, _ := registryTestSetup(t)

	err := svc.DeleteRegistry("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFallbackRepositories_ReturnsNamespaceQualifiedPaths(t *testing.T) {
	svc, db := registryTestSetup(t)

	created, err := svc.CreateRegistry(CreateRegistryReq{Name: "fallback", URL: "https://quay.io", Namespace: "demo"})
	if err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}

	publishedBy := uuid.New()
	for _, repository := range []string{"notebook", "demo/already-qualified"} {
		if err := db.Create(&models.Publication{
			WorkspaceID:   uuid.New(),
			VersionNumber: 1,
			RegistryID:    created.ID,
			Repository:    repository,
			Tag:           "v1",
			PublishedBy:   publishedBy,
		}).Error; err != nil {
			t.Fatalf("create publication: %v", err)
		}
	}

	repositories := svc.FallbackRepositories(created.ID.String())
	seen := make(map[string]bool, len(repositories))
	for _, repository := range repositories {
		seen[repository] = true
	}
	for _, want := range []string{"demo/notebook", "demo/already-qualified"} {
		if !seen[want] {
			t.Fatalf("expected fallback repository %q in %v", want, repositories)
		}
	}
	if seen["demo/demo/already-qualified"] {
		t.Fatalf("did not expect namespace to be duplicated in %v", repositories)
	}
}
