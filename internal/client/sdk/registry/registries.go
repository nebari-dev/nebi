package registry

// Owner: U7 (publish). Registries page, add/remove registry, capability.

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// ListRegistries returns configured registries with capabilities.
func (s *Service) ListRegistries(ctx context.Context) ([]shared.RegistryItem, error) {
	return nil, shared.NotImplemented("registry.ListRegistries")
}

// AddRegistry stores a registry; the password goes to the keyring.
func (s *Service) AddRegistry(ctx context.Context, req shared.AddRegistryRequest) (*shared.RegistryItem, error) {
	return nil, shared.NotImplemented("registry.AddRegistry")
}

// RemoveRegistry deletes a registry and its keyring password.
func (s *Service) RemoveRegistry(ctx context.Context, id uuid.UUID) error {
	return shared.NotImplemented("registry.RemoveRegistry")
}
