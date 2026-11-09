package core

// Owner: U7 (publish). Publication metadata on the server (gap 2).

import (
	"context"

	"github.com/google/uuid"
)

// RecordPublication stores publication metadata for a version.
func (s *Service) RecordPublication(ctx context.Context, userID, projectID uuid.UUID, version int, rec PublicationRecord) (*PublicationRecord, error) {
	return nil, notImplemented("core.RecordPublication")
}

// RemovePublication marks a publication unpublished.
func (s *Service) RemovePublication(ctx context.Context, userID, projectID uuid.UUID, version int, publicationID uuid.UUID) error {
	return notImplemented("core.RemovePublication")
}

// PublicationsByVersion returns active publication records keyed by
// version number (used by ListSyncVersions).
func (s *Service) PublicationsByVersion(ctx context.Context, projectID uuid.UUID) (map[int][]PublicationRecord, error) {
	return map[int][]PublicationRecord{}, nil
}
