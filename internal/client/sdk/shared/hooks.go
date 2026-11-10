package shared

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

type noJobs struct{}

func (noJobs) Submit(context.Context, JobRequest) (*models.Job, error) {
	return nil, NotImplemented("job submitter")
}

// ProjectDecorator fills project-list fields owned by another unit. The
// owner of the base list (sdk/local) runs every registered decorator, in
// registration order, after building the items from the local database.
// Registered in sdk.New and internal/client.NewStack:
//   - sdk/remote: UpdateAvailable, server entries of Remotes
//   - internal/client/jobs: FailedInstallJobID
//
// Decorators must not fail the list when the server is unreachable: leave
// the fields at their zero value and return nil.
type ProjectDecorator interface {
	DecorateProjects(ctx context.Context, items []ProjectListItem) error
}

// VersionDecorator fills version fields owned by another unit and may
// append items (server-only versions). Registered in sdk.New:
//   - sdk/registry: Published (local publication records)
//   - sdk/remote: Sync markers, server-only versions, server publications
type VersionDecorator interface {
	DecorateVersions(ctx context.Context, projectID uuid.UUID, items []VersionItem) ([]VersionItem, error)
}
