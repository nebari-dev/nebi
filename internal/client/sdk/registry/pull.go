package registry

// Owner: U7 (publish). Pull a project from a registry.

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

// ListRepositories lists repositories in a registry namespace.
func (s *Service) ListRepositories(ctx context.Context, registryID uuid.UUID, search string) ([]string, error) {
	return nil, shared.NotImplemented("registry.ListRepositories")
}

// ListTags lists tags of a repository.
func (s *Service) ListTags(ctx context.Context, registryID uuid.UUID, repository string) ([]string, error) {
	return nil, shared.NotImplemented("registry.ListTags")
}

// Pull creates the local project (status pending) with a registry
// ProjectRemote and submits a JobTypePullRegistry job.
func (s *Service) Pull(ctx context.Context, registryID uuid.UUID, req shared.RegistryPullRequest) (*shared.PullResult, error) {
	return nil, shared.NotImplemented("registry.Pull")
}

// RunPull is the pull_registry job body.
func (s *Service) RunPull(ctx context.Context, job *models.Job, log io.Writer) error {
	return shared.NotImplemented("registry.RunPull")
}
