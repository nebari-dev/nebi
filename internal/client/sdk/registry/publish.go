package registry

// Owner: U7 (publish). Publish / unpublish and the local Published badge.

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// PublishDefaults returns the prefilled repository:tag per registry.
func (s *Service) PublishDefaults(ctx context.Context, projectID uuid.UUID, version int) ([]shared.PublishDefault, error) {
	return nil, shared.NotImplemented("registry.PublishDefaults")
}

// Publish pushes a version to each target and records a models.Publication
// per success (RegistryHost filled). Synchronous; one result per target.
// Also reports publications to the linked server when connected.
func (s *Service) Publish(ctx context.Context, projectID uuid.UUID, version int, req shared.PublishRequest) (*shared.PublishResult, error) {
	return nil, shared.NotImplemented("registry.Publish")
}

// Unpublish deletes the tag from the registry when it supports tag
// deletion (else shared.ErrUnavailable) and soft-deletes the publication.
func (s *Service) Unpublish(ctx context.Context, projectID uuid.UUID, publicationID uuid.UUID) error {
	return shared.NotImplemented("registry.Unpublish")
}

// DecorateVersions adds local publications to VersionItem.Published.
// Registered in sdk.New.
func (s *Service) DecorateVersions(ctx context.Context, projectID uuid.UUID, items []shared.VersionItem) ([]shared.VersionItem, error) {
	return items, nil
}
