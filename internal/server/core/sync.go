package core

// Owner: U5 (remote). Project catalog and version sync.

import (
	"context"

	"github.com/google/uuid"
)

// ListCatalogProjects lists projects the caller can read, with the caller's
// role, filtered by a case-insensitive name search.
func (s *Service) ListCatalogProjects(ctx context.Context, userID uuid.UUID, search string) ([]CatalogProject, error) {
	return nil, notImplemented("core.ListCatalogProjects")
}

// CreateCatalogProject creates an empty, ready server project owned by the
// caller (the first push of a local project).
func (s *Service) CreateCatalogProject(ctx context.Context, userID uuid.UUID, req CreateCatalogProjectRequest) (*CatalogProject, error) {
	return nil, notImplemented("core.CreateCatalogProject")
}

// ListSyncVersions lists a project's versions (oldest first) with tags and
// publication records.
func (s *Service) ListSyncVersions(ctx context.Context, projectID uuid.UUID) ([]SyncVersion, error) {
	return nil, notImplemented("core.ListSyncVersions")
}

// GetSyncVersion returns one version with its files.
func (s *Service) GetSyncVersion(ctx context.Context, projectID uuid.UUID, number int) (*SyncVersionFiles, error) {
	return nil, notImplemented("core.GetSyncVersion")
}

// PushVersions stores pushed versions with their numbers, moves tags
// ("latest" always to the newest) and records publications. A number that
// exists with the same content hash is accepted idempotently; with other
// content it is a ConflictError "version_conflict".
func (s *Service) PushVersions(ctx context.Context, userID, projectID uuid.UUID, req PushVersionsRequest) (*PushVersionsResult, error) {
	return nil, notImplemented("core.PushVersions")
}
