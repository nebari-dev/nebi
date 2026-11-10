package local

// Owner: U7 (publish). Registry passwords live in the OS keyring
// (Deps.Credentials), keyed by registry name; never in SQLite.

import (
	"context"
	"fmt"
	"strings"
)

// SetRegistryPassword stores a registry password. An empty password removes
// the stored one.
func (s *Service) SetRegistryPassword(ctx context.Context, registryName, password string) error {
	if strings.TrimSpace(registryName) == "" {
		return fmt.Errorf("registry name is required")
	}
	if password == "" {
		return s.DeleteRegistryPassword(ctx, registryName)
	}
	if s.deps.Credentials == nil {
		return fmt.Errorf("no credential store configured")
	}
	if err := s.deps.Credentials.SetPassword(registryName, password); err != nil {
		return fmt.Errorf("storing the password for registry %q: %w", registryName, err)
	}
	return nil
}

// RegistryPassword returns a registry password ("" when none is stored).
// Credential stores report a missing entry as an error, so any lookup
// failure is treated as "no password": the registry is then used
// anonymously and reports its own authentication error if it needs one.
func (s *Service) RegistryPassword(ctx context.Context, registryName string) (string, error) {
	if s.deps.Credentials == nil || strings.TrimSpace(registryName) == "" {
		return "", nil
	}
	pw, err := s.deps.Credentials.GetPassword(registryName)
	if err != nil {
		return "", nil
	}
	return pw, nil
}

// DeleteRegistryPassword removes a registry password. Removing a password
// that is not stored is not an error.
func (s *Service) DeleteRegistryPassword(ctx context.Context, registryName string) error {
	if s.deps.Credentials == nil || strings.TrimSpace(registryName) == "" {
		return nil
	}
	if _, err := s.deps.Credentials.GetPassword(registryName); err != nil {
		return nil
	}
	if err := s.deps.Credentials.DeletePassword(registryName); err != nil {
		return fmt.Errorf("removing the password for registry %q: %w", registryName, err)
	}
	return nil
}
