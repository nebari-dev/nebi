package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

func reconcileTestSetup(t *testing.T) *gorm.DB {
	t.Helper()
	_, db := testSetup(t, false)
	return db
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
	db := reconcileTestSetup(t)

	err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com", Namespace: "acme-envs", Default: true, Restricted: true},
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
	if !reg.Restricted {
		t.Error("expected restricted=true")
	}
	// Config-managed registries are public-only: no credentials stored.
	if reg.Username != "" || reg.Password != "" || reg.APIToken != "" {
		t.Errorf("expected empty credentials, got username=%q password=%q api_token=%q",
			reg.Username, reg.Password, reg.APIToken)
	}
}

func TestReconcile_UpdatesExisting(t *testing.T) {
	db := reconcileTestSetup(t)

	entries := []config.RegistryEntryConfig{{Name: "acme", URL: "old.acme.com", Namespace: "old-envs"}}
	if err := ReconcileConfigRegistries(db, entries); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	entries[0].URL = "new.acme.com"
	entries[0].Namespace = "new-envs"
	entries[0].Restricted = true
	if err := ReconcileConfigRegistries(db, entries); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	reg := getRegistryByName(t, db, "acme")
	if reg.URL != "new.acme.com" || reg.Namespace != "new-envs" || !reg.Restricted {
		t.Errorf("expected updated fields, got url=%q namespace=%q restricted=%v", reg.URL, reg.Namespace, reg.Restricted)
	}

	var count int64
	db.Model(&models.OCIRegistry{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 registry, got %d", count)
	}
}

func TestReconcile_TakesOverUserCreated(t *testing.T) {
	db := reconcileTestSetup(t)

	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "acme", URL: "user.acme.com", Username: "svc", Password: "enc:v1:abc", APIToken: "enc:v1:def"})

	err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
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
	// Public-only: takeover clears any credentials the user had stored.
	if reg.Username != "" || reg.Password != "" || reg.APIToken != "" {
		t.Errorf("expected takeover to clear credentials, got username=%q password=%q api_token=%q",
			reg.Username, reg.Password, reg.APIToken)
	}
}

func TestReconcile_RemovesStale(t *testing.T) {
	db := reconcileTestSetup(t)

	// A user-created registry that must survive.
	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "personal", URL: "personal.io"})

	if err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	// Entry removed from config entirely.
	if err := ReconcileConfigRegistries(db, nil); err != nil {
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
	db := reconcileTestSetup(t)

	// Existing user-created default.
	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "personal", URL: "personal.io", IsDefault: true})

	if err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
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
	db := reconcileTestSetup(t)

	db.Create(&models.OCIRegistry{ID: uuid.New(), Name: "personal", URL: "personal.io", IsDefault: true})

	if err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com"},
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if !getRegistryByName(t, db, "personal").IsDefault {
		t.Error("existing default must be untouched when no config entry claims default")
	}
}

func TestReconcile_RemoveThenReAdd(t *testing.T) {
	db := reconcileTestSetup(t)

	if err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := ReconcileConfigRegistries(db, nil); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	if err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
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
	db := reconcileTestSetup(t)

	if err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com", Default: true},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if !getRegistryByName(t, db, "acme").IsDefault {
		t.Fatal("expected acme to be default after first reconcile")
	}

	if err := ReconcileConfigRegistries(db, []config.RegistryEntryConfig{
		{Name: "acme", URL: "registry.acme.com", Default: false},
	}); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	if getRegistryByName(t, db, "acme").IsDefault {
		t.Error("expected acme to no longer be default once its entry sets default=false")
	}
}

func TestIsDuplicateKeyErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sqlite unique constraint",
			err:  errors.New("UNIQUE constraint failed: oci_registries.name"),
			want: true,
		},
		{
			name: "postgres duplicate key",
			err:  errors.New("duplicate key value violates unique constraint \"oci_registries_name_key\""),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDuplicateKeyErr(tt.err); got != tt.want {
				t.Errorf("isDuplicateKeyErr(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
