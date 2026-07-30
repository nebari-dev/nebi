package service

import (
	"errors"
	"fmt"
	"log/slog"

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
//     over by config (config is the source of truth)
//   - an entry with Default=true becomes the single default registry;
//     when no entry claims default, existing default state is untouched
//
// Credentials are encrypted with the same key the registry service uses.
func ReconcileConfigRegistries(db *gorm.DB, encKey []byte, entries []config.RegistryEntryConfig) error {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}

	// Remove config-managed rows dropped from config. Same hard-delete
	// semantics as an admin delete; publications keep their registry ID.
	stale := db.Where("config_managed = ?", true)
	if len(names) > 0 {
		stale = stale.Where("name NOT IN ?", names)
	}
	if err := stale.Delete(&models.OCIRegistry{}).Error; err != nil {
		return fmt.Errorf("remove stale config-managed registries: %w", err)
	}

	for _, e := range entries {
		encPassword, err := nebicrypto.EncryptField(e.Password, encKey)
		if err != nil {
			return fmt.Errorf("encrypt credentials for registry %q: %w", e.Name, err)
		}
		encAPIToken, err := nebicrypto.EncryptField(e.APIToken, encKey)
		if err != nil {
			return fmt.Errorf("encrypt credentials for registry %q: %w", e.Name, err)
		}

		var row models.OCIRegistry
		err = db.Where("name = ?", e.Name).First(&row).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			row = models.OCIRegistry{
				Name:          e.Name,
				URL:           e.URL,
				Username:      e.Username,
				Password:      encPassword,
				APIToken:      encAPIToken,
				IsDefault:     e.Default,
				Namespace:     e.Namespace,
				ConfigManaged: true,
			}
			if err := db.Create(&row).Error; err != nil {
				return fmt.Errorf("create config-managed registry %q: %w", e.Name, err)
			}
			slog.Info("Created config-managed registry", "name", e.Name, "url", e.URL)
		case err != nil:
			return fmt.Errorf("look up registry %q: %w", e.Name, err)
		default:
			if !row.ConfigManaged {
				slog.Warn("Config takes over existing user-created registry", "name", e.Name)
			}
			row.URL = e.URL
			row.Username = e.Username
			row.Password = encPassword
			row.APIToken = encAPIToken
			row.IsDefault = e.Default
			row.Namespace = e.Namespace
			row.ConfigManaged = true
			if err := db.Save(&row).Error; err != nil {
				return fmt.Errorf("update config-managed registry %q: %w", e.Name, err)
			}
		}

		if e.Default {
			if err := db.Model(&models.OCIRegistry{}).
				Where("is_default = ? AND id != ?", true, row.ID).
				Update("is_default", false).Error; err != nil {
				return fmt.Errorf("clear previous default registry: %w", err)
			}
		}
	}

	return nil
}
