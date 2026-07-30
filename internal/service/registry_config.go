package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nebari-dev/nebi/internal/config"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
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
// Credentials are encrypted with the same key the registry service uses.
func ReconcileConfigRegistries(db *gorm.DB, encKey []byte, entries []config.RegistryEntryConfig) error {
	// Encrypt everything up front so a bad key fails fast, before any
	// writes (stale-delete or upserts) happen.
	names := make([]string, 0, len(entries))
	encrypted := make([]encryptedRegistryEntry, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)

		encPassword, err := nebicrypto.EncryptField(e.Password, encKey)
		if err != nil {
			return fmt.Errorf("encrypt credentials for registry %q: %w", e.Name, err)
		}
		encAPIToken, err := nebicrypto.EncryptField(e.APIToken, encKey)
		if err != nil {
			return fmt.Errorf("encrypt credentials for registry %q: %w", e.Name, err)
		}
		encrypted = append(encrypted, encryptedRegistryEntry{entry: e, password: encPassword, apiToken: encAPIToken})
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

		for _, ee := range encrypted {
			if err := reconcileRegistryEntry(tx, ee); err != nil {
				return err
			}
		}

		return nil
	})
}

// encryptedRegistryEntry pairs a config entry with its already-encrypted
// credential fields.
type encryptedRegistryEntry struct {
	entry    config.RegistryEntryConfig
	password string
	apiToken string
}

func reconcileRegistryEntry(tx *gorm.DB, ee encryptedRegistryEntry) error {
	e := ee.entry

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
		if err := updateManagedRegistryRow(tx, &row, ee); err != nil {
			return err
		}

	case errors.Is(err, gorm.ErrRecordNotFound):
		row = models.OCIRegistry{
			Name:          e.Name,
			URL:           e.URL,
			Username:      e.Username,
			Password:      ee.password,
			APIToken:      ee.apiToken,
			IsDefault:     e.Default,
			Namespace:     e.Namespace,
			ConfigManaged: true,
		}
		createErr := tx.Create(&row).Error
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
			if err := updateManagedRegistryRow(tx, &row, ee); err != nil {
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

func updateManagedRegistryRow(tx *gorm.DB, row *models.OCIRegistry, ee encryptedRegistryEntry) error {
	e := ee.entry
	row.URL = e.URL
	row.Username = e.Username
	row.Password = ee.password
	row.APIToken = ee.apiToken
	row.IsDefault = e.Default
	row.Namespace = e.Namespace
	row.ConfigManaged = true
	// Every boot re-encrypts and saves each managed row (a fresh AES-GCM
	// nonce each time), so updated_at churns even when nothing actually
	// changed. Accepted: reconciliation stays simple and declarative
	// rather than diffing fields to skip a no-op write.
	if err := tx.Save(row).Error; err != nil {
		return fmt.Errorf("update config-managed registry %q: %w", e.Name, err)
	}
	return nil
}

// isDuplicateKeyErr detects a unique-constraint violation the same way
// RegistryService.CreateRegistry does.
func isDuplicateKeyErr(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate")
}
