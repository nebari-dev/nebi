package store

import (
	"fmt"

	"github.com/google/uuid"
)

// CreateRegistry creates a new local registry record.
func (s *Store) CreateRegistry(reg *LocalRegistry) error {
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
