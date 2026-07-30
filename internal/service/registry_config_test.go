package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

func reconcileTestSetup(t *testing.T) (*gorm.DB, []byte) {
	t.Helper()
	_, db := testSetup(t, false)
	encKey := []byte("test-encryption-key-32bytes!!!!!")
	return db, encKey
}

func getRegistryByName(t *testing.T, db *gorm.DB, name string) *models.OCIRegistry {
	t.Helper()
	var reg models.OCIRegistry
	if err := db.Where("name = ?", name).First(&reg).Error; err != nil {
		return nil
	}
	return &reg
}

func TestReconcile_CreatesEntries(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com", Namespace: "acme-envs", Username: "svc", Password: "hunter2", APIToken: "tok-123", Default: true},
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	reg := getRegistryByName(t, db, "acme")
	if reg == nil {
		t.Fatal("expected registry to be created")
	}
	if !reg.ConfigManaged {
		t.Error("expected config_managed=true")
	}
	if !reg.IsDefault {
		t.Error("expected is_default=true")
	}
	// Credentials must be encrypted at rest and decryptable.
	if reg.Password == "hunter2" {
		t.Error("password stored in plaintext")
	}
	plain, err := nebicrypto.DecryptField(reg.Password, encKey)
	if err != nil || plain != "hunter2" {
		t.Errorf("expected decryptable password, got %q err=%v", plain, err)
	}
	if reg.APIToken == "tok-123" {
		t.Error("api token stored in plaintext")
	}
	plainToken, err := nebicrypto.DecryptField(reg.APIToken, encKey)
	if err != nil || plainToken != "tok-123" {
		t.Errorf("expected decryptable api token, got %q err=%v", plainToken, err)
	}
}

func TestReconcile_UpdatesExisting(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	entries := []config.RegistryEntryConfig{{Name: "acme", URL: "old.acme.com", Username: "old"}}
	if err := ReconcileConfigRegistries(db, encKey, entries); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	entries[0].URL = "new.acme.com"
	entries[0].Username = "new"
	if err := ReconcileConfigRegistries(db, encKey, entries); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	reg := getRegistryByName(t, db, "acme")
	if reg.URL != "new.acme.com" || reg.Username != "new" {
		t.Errorf("expected updated fields, got url=%q username=%q", reg.URL, reg.Username)
	}

	var count int64
	db.Model(&models.OCIRegistry{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 registry, got %d", count)
	}
}

func TestReconcile_TakesOverUserCreated(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "acme", URL: "user.acme.com"})

	err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "config.acme.com"},
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	reg := getRegistryByName(t, db, "acme")
	if !reg.ConfigManaged {
		t.Error("expected takeover to mark registry config-managed")
	}
	if reg.URL != "config.acme.com" {
		t.Errorf("expected config to win, got url=%q", reg.URL)
	}
}

func TestReconcile_RemovesStale(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	// A user-created registry that must survive.
	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "personal", URL: "personal.io"})

	if err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	// Entry removed from config entirely.
	if err := ReconcileConfigRegistries(db, encKey, nil); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	if getRegistryByName(t, db, "acme") != nil {
		t.Error("expected config-managed registry to be removed when dropped from config")
	}
	if getRegistryByName(t, db, "personal") == nil {
		t.Error("user-created registry must not be removed")
	}
}

func TestReconcile_SwitchesDefault(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	// Existing user-created default.
	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "personal", URL: "personal.io", IsDefault: true})

	if err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com", Default: true},
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if !getRegistryByName(t, db, "acme").IsDefault {
		t.Error("expected acme to be default")
	}
	if getRegistryByName(t, db, "personal").IsDefault {
		t.Error("expected previous default to be cleared")
	}
}

func TestReconcile_NoDefaultClaimLeavesExisting(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "personal", URL: "personal.io", IsDefault: true})

	if err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com"},
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if !getRegistryByName(t, db, "personal").IsDefault {
		t.Error("existing default must be untouched when no config entry claims default")
	}
}

func TestReconcile_RemoveThenReAdd(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	if err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := ReconcileConfigRegistries(db, encKey, nil); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	if err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com"},
	}); err != nil {
		t.Fatalf("third reconcile: %v", err)
	}

	if getRegistryByName(t, db, "acme") == nil {
		t.Error("expected acme to exist after remove-then-re-add")
	}

	// Guards against the stale-delete becoming a soft delete (e.g. if
	// OCIRegistry.DeletedAt were ever changed to gorm.DeletedAt): an
	// unscoped count must show exactly one row, not a live one plus a
	// soft-deleted leftover.
	var count int64
	db.Unscoped().Model(&models.OCIRegistry{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 registry (unscoped), got %d", count)
	}
}

func TestReconcile_DefaultTrueThenFalse(t *testing.T) {
	db, encKey := reconcileTestSetup(t)

	if err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com", Default: true},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if !getRegistryByName(t, db, "acme").IsDefault {
		t.Fatal("expected acme to be default after first reconcile")
	}

	if err := ReconcileConfigRegistries(db, encKey, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com", Default: false},
	}); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	if getRegistryByName(t, db, "acme").IsDefault {
		t.Error("expected acme to no longer be default once its entry sets default=false")
	}
}
