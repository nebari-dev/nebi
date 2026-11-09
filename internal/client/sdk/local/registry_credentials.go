package local

// Owner: U7 (publish). Registry passwords live in the OS keyring
// (Deps.Credentials), keyed by registry name; never in SQLite.

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// SetRegistryPassword stores a registry password.
func (s *Service) SetRegistryPassword(ctx context.Context, registryName, password string) error {
	return shared.NotImplemented("local.SetRegistryPassword")
}

// RegistryPassword returns a registry password ("" when none is stored).
func (s *Service) RegistryPassword(ctx context.Context, registryName string) (string, error) {
	return "", shared.NotImplemented("local.RegistryPassword")
}

// DeleteRegistryPassword removes a registry password.
func (s *Service) DeleteRegistryPassword(ctx context.Context, registryName string) error {
	return shared.NotImplemented("local.DeleteRegistryPassword")
}
