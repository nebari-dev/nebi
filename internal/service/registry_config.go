package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// ReconcileConfigRegistries makes the oci_registries table match the
// registries.entries section of config.yaml. It runs at boot in both team
// and local mode:
//
//   - each entry is upserted by its unique name and marked config-managed
//   - config-managed rows whose name is no longer in config are deleted
//   - a user-created registry whose name collides with an entry is taken
//     over by config (config is the source of truth); this is declarative,
//     so an entry with Default=false clears the default flag even when the
//     row being taken over was the current default, potentially leaving no
//     default registry at all until a later config change sets one
//   - an entry with Default=true becomes the single default registry;
//     when no entry claims default, existing default state is untouched
//
// The api and worker processes can boot concurrently against the same
// config, so the whole reconciliation runs in a single transaction and the
// create path tolerates a duplicate-name error as a sign that the other
// process won the race to create the row; it falls back to updating that
// row instead of failing boot.
//
// Config-managed registries are credentialless, so rows never carry
// credentials; a takeover clears any the user had stored.
func ReconcileConfigRegistries(db *gorm.DB, entries []config.RegistryEntryConfig) error {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Remove config-managed rows dropped from config. Same hard-delete
		// semantics as an admin delete; publications keep their registry ID.
		stale := tx.Where("config_managed = ?", true)
		if len(names) > 0 {
			stale = stale.Where("name NOT IN ?", names)
		}
		if err := stale.Delete(&models.OCIRegistry{}).Error; err != nil {
			return fmt.Errorf("remove stale config-managed registries: %w", err)
		}

		for _, e := range entries {
			if err := reconcileRegistryEntry(tx, e); err != nil {
				return err
			}
		}

		return nil
	})
}

func reconcileRegistryEntry(tx *gorm.DB, e config.RegistryEntryConfig) error {
	var row models.OCIRegistry
	err := tx.Where("name = ?", e.Name).First(&row).Error

	switch {
	case err == nil:
		// Existing row: either already config-managed, or a user-created
		// row whose name collides with this entry (takeover).
		if !row.ConfigManaged {
			slog.Warn("Config takes over existing user-created registry", "name", e.Name)
		}
		if row.IsDefault && !e.Default {
			slog.Warn("Config takeover clears default flag on registry", "name", e.Name)
		}
		if err := updateManagedRegistryRow(tx, &row, e); err != nil {
			return err
		}

	case errors.Is(err, gorm.ErrRecordNotFound):
		row = models.OCIRegistry{
			Name:          e.Name,
			URL:           e.URL,
			IsDefault:     e.Default,
			Namespace:     e.Namespace,
			ConfigManaged: true,
			Restricted:    e.Restricted,
		}
		// Run the create under a savepoint: on Postgres a failed statement
		// aborts the enclosing transaction (SQLSTATE 25P02), so a plain
		// tx.Create failing on the unique-name constraint would poison the
		// outer transaction and make the fallback lookup below fail too.
		// gorm emits SAVEPOINT/ROLLBACK TO SAVEPOINT for a nested
		// Transaction call on an existing tx, so only this statement is
		// rolled back and the outer transaction survives.
		createErr := tx.Transaction(func(tx2 *gorm.DB) error {
			return tx2.Create(&row).Error
		})
		switch {
		case createErr == nil:
			slog.Info("Created config-managed registry", "name", e.Name, "url", e.URL)
		case isDuplicateKeyErr(createErr):
			// api and worker can boot concurrently against identical
			// config: both saw ErrRecordNotFound and raced to create this
			// row. The loser hits the unique-name constraint here; treat
			// that as "the row now exists" and fall back to updating it
			// instead of crashing boot.
			slog.Info("Registry created concurrently by another process, updating instead", "name", e.Name)
			if err := tx.Where("name = ?", e.Name).First(&row).Error; err != nil {
				return fmt.Errorf("look up concurrently created registry %q: %w", e.Name, err)
			}
			if err := updateManagedRegistryRow(tx, &row, e); err != nil {
				return err
			}
		default:
			return fmt.Errorf("create config-managed registry %q: %w", e.Name, createErr)
		}

	default:
		return fmt.Errorf("look up registry %q: %w", e.Name, err)
	}

	if e.Default {
		if err := tx.Model(&models.OCIRegistry{}).
			Where("is_default = ? AND id != ?", true, row.ID).
			Update("is_default", false).Error; err != nil {
			return fmt.Errorf("clear previous default registry: %w", err)
		}
	}

	return nil
}

func updateManagedRegistryRow(tx *gorm.DB, row *models.OCIRegistry, e config.RegistryEntryConfig) error {
	row.URL = e.URL
	row.Username = ""
	row.Password = ""
	row.APIToken = ""
	row.IsDefault = e.Default
	row.Namespace = e.Namespace
	row.ConfigManaged = true
	row.Restricted = e.Restricted
	// Every boot saves each managed row, so updated_at churns even when
	// nothing actually changed. Accepted: reconciliation stays simple and
	// declarative rather than diffing fields to skip a no-op write.
	if err := tx.Save(row).Error; err != nil {
		return fmt.Errorf("update config-managed registry %q: %w", e.Name, err)
	}
	return nil
}

// isDuplicateKeyErr detects a unique-constraint violation the same way
// RegistryService.CreateRegistry does. err must be non-nil; callers only
// invoke this on a known create error.
func isDuplicateKeyErr(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate")
}
