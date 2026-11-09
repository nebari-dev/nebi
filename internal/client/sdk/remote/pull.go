package remote

// Owner: U5 (remote). Pull a server project (optionally installing it) and
// pull a server-only version.

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

// Pull checks the local name (ConflictError "name_taken"), creates the
// local project row (status pending) with a server ProjectRemote, and
// submits a JobTypePull or JobTypePullInstall job with metadata
// {"server_project_id", "server_url"}.
func (s *Service) Pull(ctx context.Context, serverProjectID string, req shared.PullRequest) (*shared.PullResult, error) {
	return nil, shared.NotImplemented("remote.Pull")
}

// RunPull is the pull / pull_install job body: download every server
// version (keeping server numbers, authors, tags, publications), write the
// latest files into the project directory and, for pull_install, install
// the latest version through exec.
func (s *Service) RunPull(ctx context.Context, job *models.Job, log io.Writer) error {
	return shared.NotImplemented("remote.RunPull")
}

// PullVersion stores a server-only version locally (synchronous).
func (s *Service) PullVersion(ctx context.Context, projectID uuid.UUID, number int) (*shared.VersionItem, error) {
	return nil, shared.NotImplemented("remote.PullVersion")
}
