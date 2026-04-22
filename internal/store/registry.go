package store

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateRegistry creates a new local registry record. If no default
// registry exists yet, the new one is auto-marked default so a lone
// registry doesn't force the user to re-run 'nebi registry add --default'.
func (s *Store) CreateRegistry(reg *LocalRegistry) error {
	if !reg.IsDefault {
		var count int64
		if err := s.db.Model(&LocalRegistry{}).Where("is_default = ?", true).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			reg.IsDefault = true
		}
	}
	return s.db.Create(reg).Error
}

// ListRegistries returns all local registries.
func (s *Store) ListRegistries() ([]LocalRegistry, error) {
	var regs []LocalRegistry
	if err := s.db.Find(&regs).Error; err != nil {
		return nil, fmt.Errorf("listing registries: %w", err)
	}
	return regs, nil
}

// GetRegistryByName returns a registry by name.
func (s *Store) GetRegistryByName(name string) (*LocalRegistry, error) {
	var reg LocalRegistry
	if err := s.db.Where("name = ?", name).First(&reg).Error; err != nil {
		return nil, fmt.Errorf("registry %q not found", name)
	}
	return &reg, nil
}

// GetDefaultRegistry returns the default registry (is_default=true).
func (s *Store) GetDefaultRegistry() (*LocalRegistry, error) {
	var reg LocalRegistry
	if err := s.db.Where("is_default = ?", true).First(&reg).Error; err != nil {
		return nil, fmt.Errorf("no default registry configured; run 'nebi registry add --default ...' first")
	}
	return &reg, nil
}

// DeleteRegistry removes a registry by ID (hard delete).
func (s *Store) DeleteRegistry(id uuid.UUID) error {
	return s.db.Unscoped().Where("id = ?", id).Delete(&LocalRegistry{}).Error
}

// SetDefaultRegistry marks the registry with the given name as the default
// and clears the flag on every other registry. Atomic — runs in a single
// transaction so the default slot never goes empty or double-filled.
func (s *Store) SetDefaultRegistry(name string) (*LocalRegistry, error) {
	var reg LocalRegistry
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("name = ?", name).First(&reg).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("registry %q not found", name)
			}
			return err
		}
		if err := tx.Model(&LocalRegistry{}).
			Where("name <> ? AND is_default = ?", name, true).
			Update("is_default", false).Error; err != nil {
			return err
		}
		if reg.IsDefault {
			return nil
		}
		reg.IsDefault = true
		return tx.Model(&reg).Update("is_default", true).Error
	})
	if err != nil {
		return nil, err
	}
	return &reg, nil
}
