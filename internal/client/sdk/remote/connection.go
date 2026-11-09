package remote

// Owner: U5 (remote). One server at a time; auth none or token.

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// GetConnection returns the current connection ("Signed in as").
func (s *Service) GetConnection(ctx context.Context) (*shared.ServerConnection, error) {
	return nil, shared.NotImplemented("remote.GetConnection")
}

// Connect verifies the server (GET /api/v1/version and /auth/me with the
// token) and stores the connection, replacing any previous one.
func (s *Service) Connect(ctx context.Context, req shared.ConnectRequest) (*shared.ServerConnection, error) {
	return nil, shared.NotImplemented("remote.Connect")
}

// Disconnect forgets the server. Pulled projects stay and keep working.
func (s *Service) Disconnect(ctx context.Context) error {
	return shared.NotImplemented("remote.Disconnect")
}
