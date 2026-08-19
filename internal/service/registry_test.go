package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func registryTestSetup(t *testing.T) (*RegistryService, *gorm.DB) {
	t.Helper()
	_, db := testSetup(t, false)
	// Use a test encryption key
	encKey := []byte("test-encryption-key-32bytes!!!!!")
	return NewRegistryService(db, encKey, false, rbac.NewDefaultProvider()), db
}

func grantRegistryAccessForTest(t *testing.T, db *gorm.DB, userID, regID uuid.UUID, action string) uuid.UUID {
	t.Helper()

	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	group, err := groupSvc.CreateGroup(CreateGroupRequest{Name: "registry-" + action + "-" + uuid.NewString()[:8]}, userID)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := groupSvc.AddMember(group.ID, userID, userID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := rbac.NewDefaultProvider().GrantGroupRegistryAccess(group.ID, regID, action); err != nil {
		t.Fatalf("grant registry %s: %v", action, err)
	}
	return group.ID
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

func TestRegistryListPublic_OpenRegistryVisibleWithoutGrant(t *testing.T) {
	svc, db := registryTestSetup(t)
	userID := createTestUser(t, db, "alice")

	registry, err := svc.CreateRegistry(CreateRegistryReq{Name: "open", URL: "https://ghcr.io"})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	registries, err := svc.ListPublicRegistries(userID)
	if err != nil {
		t.Fatalf("ListPublicRegistries: %v", err)
	}
	if len(registries) != 1 || registries[0].ID != registry.ID {
		t.Fatalf("expected open registry without grant, got %+v", registries)
	}
}

func TestRegistryOpenAccess_AllowsReadAndWriteWithoutGrant(t *testing.T) {
	svc, db := registryTestSetup(t)
	userID := createTestUser(t, db, "alice")

	registry, err := svc.CreateRegistry(CreateRegistryReq{
		Name:     "open-creds",
		URL:      "https://ghcr.io",
		Username: "secret-user",
		Password: "secret-pass",
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	withCreds, err := svc.GetRegistryWithCredentials(registry.ID.String(), userID)
	if err != nil {
		t.Fatalf("expected open registry read to succeed without grant: %v", err)
	}
	if withCreds.Password != "secret-pass" {
		t.Fatalf("expected decrypted password, got %q", withCreds.Password)
	}

	if err := ensureRegistryAccess(db, rbac.NewDefaultProvider(), false, userID, registry.ID, "write"); err != nil {
		t.Fatalf("expected open registry write to succeed without grant: %v", err)
	}
}

func TestRegistryListPublic_HidesCredentials(t *testing.T) {
	svc, db := registryTestSetup(t)

	registry, err := svc.CreateRegistry(CreateRegistryReq{
		Name:     "public-reg",
		URL:      "https://ghcr.io",
		Username: "secret-user",
		Password: "secret-pass",
		APIToken: "secret-token",
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	userID := createTestUser(t, db, "alice")
	grantRegistryAccessForTest(t, db, userID, registry.ID, "read")

	registries, err := svc.ListPublicRegistries(userID)
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

func TestRegistryListPublic_FiltersRestrictedUnreadableRegistries(t *testing.T) {
	svc, db := registryTestSetup(t)
	userID := createTestUser(t, db, "alice")

	readable, err := svc.CreateRegistry(CreateRegistryReq{Name: "readable", URL: "https://read.example", Restricted: true})
	if err != nil {
		t.Fatalf("create readable registry: %v", err)
	}
	if _, err := svc.CreateRegistry(CreateRegistryReq{Name: "hidden", URL: "https://hidden.example", Restricted: true}); err != nil {
		t.Fatalf("create hidden registry: %v", err)
	}

	grantRegistryAccessForTest(t, db, userID, readable.ID, "read")

	registries, err := svc.ListPublicRegistries(userID)
	if err != nil {
		t.Fatalf("ListPublicRegistries: %v", err)
	}
	if len(registries) != 1 || registries[0].ID != readable.ID {
		t.Fatalf("expected only readable registry, got %+v", registries)
	}
}

func TestGetRegistryWithCredentials_RequiresReadAccessAndHonorsRevocation(t *testing.T) {
	svc, db := registryTestSetup(t)
	userID := createTestUser(t, db, "alice")

	registry, err := svc.CreateRegistry(CreateRegistryReq{
		Name:       "private",
		URL:        "https://ghcr.io",
		Username:   "secret-user",
		Password:   "secret-pass",
		Restricted: true,
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	if _, err := svc.GetRegistryWithCredentials(registry.ID.String(), userID); err == nil {
		t.Fatal("expected forbidden error without registry read grant")
	} else if !isForbiddenError(err, nil) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}

	groupID := grantRegistryAccessForTest(t, db, userID, registry.ID, "read")

	withCreds, err := svc.GetRegistryWithCredentials(registry.ID.String(), userID)
	if err != nil {
		t.Fatalf("expected registry read to succeed after grant: %v", err)
	}
	if withCreds.Password != "secret-pass" {
		t.Fatalf("expected decrypted password, got %q", withCreds.Password)
	}

	if err := rbac.NewDefaultProvider().RevokeGroupRegistryAccess(groupID, registry.ID); err != nil {
		t.Fatalf("revoke registry read: %v", err)
	}
	if _, err := svc.GetRegistryWithCredentials(registry.ID.String(), userID); err == nil {
		t.Fatal("expected forbidden error after registry grant revocation")
	} else if !isForbiddenError(err, nil) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
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

func TestRegistryList_IncludesConfigManaged(t *testing.T) {
	svc, db := registryTestSetup(t)

	db.Create(&models.OCIRegistry{
		ID:            uuid.New(),
		Name:          "managed",
		URL:           "registry.acme.com",
		ConfigManaged: true,
	})

	registries, err := svc.ListRegistries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(registries) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(registries))
	}
	if !registries[0].ConfigManaged {
		t.Error("expected config_managed to be true in result")
	}
}

func TestRegistryUpdate_ConfigManagedRejected(t *testing.T) {
	svc, db := registryTestSetup(t)

	reg := models.OCIRegistry{ID: uuid.New(), Name: "managed", URL: "a.io", ConfigManaged: true}
	db.Create(&reg)

	newURL := "b.io"
	_, err := svc.UpdateRegistry(reg.ID.String(), UpdateRegistryReq{URL: &newURL})
	if err == nil {
		t.Fatal("expected error updating config-managed registry")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestRegistryDelete_ConfigManagedRejected(t *testing.T) {
	svc, db := registryTestSetup(t)

	reg := models.OCIRegistry{ID: uuid.New(), Name: "managed", URL: "a.io", ConfigManaged: true}
	db.Create(&reg)

	err := svc.DeleteRegistry(reg.ID.String())
	if err == nil {
		t.Fatal("expected error deleting config-managed registry")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestRegistryCreate_CannotStealConfigManagedDefault(t *testing.T) {
	svc, db := registryTestSetup(t)

	managed := models.OCIRegistry{ID: uuid.New(), Name: "managed", URL: "a.io", ConfigManaged: true, IsDefault: true}
	db.Create(&managed)

	_, err := svc.CreateRegistry(CreateRegistryReq{Name: "challenger", URL: "b.io", IsDefault: true})
	if err == nil {
		t.Fatal("expected error stealing config-managed default")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	var reloaded models.OCIRegistry
	if err := db.Where("id = ?", managed.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload managed registry: %v", err)
	}
	if !reloaded.IsDefault {
		t.Error("expected managed registry to remain default")
	}
}

func TestRegistryUpdate_CannotStealConfigManagedDefault(t *testing.T) {
	svc, db := registryTestSetup(t)

	managed := models.OCIRegistry{ID: uuid.New(), Name: "managed", URL: "a.io", ConfigManaged: true, IsDefault: true}
	db.Create(&managed)

	normal, err := svc.CreateRegistry(CreateRegistryReq{Name: "normal", URL: "b.io"})
	if err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}

	newDefault := true
	_, err = svc.UpdateRegistry(normal.ID.String(), UpdateRegistryReq{IsDefault: &newDefault})
	if err == nil {
		t.Fatal("expected error stealing config-managed default")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	var reloaded models.OCIRegistry
	if err := db.Where("id = ?", managed.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload managed registry: %v", err)
	}
	if !reloaded.IsDefault {
		t.Error("expected managed registry to remain default")
	}
}
