package local

// Owner: U5 (remote). The server token lives in store_credentials (one
// server at a time, see store.Config / store.Credentials).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// ServerCredentials is the stored connection to the team server.
type ServerCredentials struct {
	URL      string
	Name     string
	Token    string
	Username string
	AuthMode string // "none" | "token"
}

// LoadServerCredentials returns the stored connection; URL is "" when not
// connected.
func (s *Service) LoadServerCredentials(ctx context.Context) (*ServerCredentials, error) {
	return nil, shared.NotImplemented("local.LoadServerCredentials")
}

// SaveServerCredentials stores (replaces) the connection.
func (s *Service) SaveServerCredentials(ctx context.Context, c ServerCredentials) error {
	return shared.NotImplemented("local.SaveServerCredentials")
}

// ClearServerCredentials removes the connection. Project remotes stay.
func (s *Service) ClearServerCredentials(ctx context.Context) error {
	return shared.NotImplemented("local.ClearServerCredentials")
}
