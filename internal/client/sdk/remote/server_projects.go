package remote

// Owner: U5 (remote). Server project list, search, refresh, details.

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// ListServerProjects lists server projects (GET /api/v1/catalog/projects).
// refresh=true records the time in store_config.last_synced_at.
func (s *Service) ListServerProjects(ctx context.Context, search string, refresh bool) (*shared.ServerProjectList, error) {
	return nil, shared.NotImplemented("remote.ListServerProjects")
}

// GetRemoteDetails is the "Remote details" view of a server project.
func (s *Service) GetRemoteDetails(ctx context.Context, serverProjectID string) (*shared.RemoteDetails, error) {
	return nil, shared.NotImplemented("remote.GetRemoteDetails")
}
