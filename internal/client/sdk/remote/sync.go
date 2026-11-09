package remote

// Owner: U5 (remote). Push, sync markers, update available, server-only
// versions, server version files (used by compare, U4).

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// Push uploads local versions the linked server does not have, creating
// the server project on first push (synchronous).
func (s *Service) Push(ctx context.Context, projectID uuid.UUID) (*shared.PushResult, error) {
	return nil, shared.NotImplemented("remote.Push")
}

// ServerVersionFiles returns pixi.toml and pixi.lock of a version on the
// linked server (for compare against a server-only version).
func (s *Service) ServerVersionFiles(ctx context.Context, projectID uuid.UUID, number int) (pixiToml, pixiLock string, err error) {
	return "", "", shared.NotImplemented("remote.ServerVersionFiles")
}

// ServerVersion returns a version that exists on the linked server
// (server_only items included) as a VersionDetail.
func (s *Service) ServerVersion(ctx context.Context, projectID uuid.UUID, number int) (*shared.VersionDetail, error) {
	return nil, shared.NotImplemented("remote.ServerVersion")
}

// DecorateProjects fills UpdateAvailable, ServerLatestVersion and the server
// entry of Remotes. Registered in sdk.New. Must not fail when the server is
// unreachable or not connected.
func (s *Service) DecorateProjects(ctx context.Context, items []shared.ProjectListItem) error {
	return nil
}

// DecorateVersions sets Sync markers, appends server-only versions and
// server publications. Registered in sdk.New. Must not fail when the server
// is unreachable or not connected.
func (s *Service) DecorateVersions(ctx context.Context, projectID uuid.UUID, items []shared.VersionItem) ([]shared.VersionItem, error) {
	return items, nil
}
