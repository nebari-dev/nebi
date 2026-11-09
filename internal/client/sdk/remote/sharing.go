package remote

// Owner: U6 (sharing). The share dialog in the client, proxied to the
// server's access endpoints (internal/server/api routes_sharing.go).

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// ListAccess returns who has access to a server project.
func (s *Service) ListAccess(ctx context.Context, serverProjectID string) (*shared.AccessList, error) {
	return nil, shared.NotImplemented("remote.ListAccess")
}

// GrantAccess adds people or teams with one role.
func (s *Service) GrantAccess(ctx context.Context, serverProjectID string, req shared.GrantAccessRequest) (*shared.AccessList, error) {
	return nil, shared.NotImplemented("remote.GrantAccess")
}

// UpdateAccess changes a principal's role.
func (s *Service) UpdateAccess(ctx context.Context, serverProjectID string, p shared.PrincipalRef, req shared.UpdateAccessRequest) (*shared.AccessList, error) {
	return nil, shared.NotImplemented("remote.UpdateAccess")
}

// RevokeAccess removes a principal's access.
func (s *Service) RevokeAccess(ctx context.Context, serverProjectID string, p shared.PrincipalRef) (*shared.AccessList, error) {
	return nil, shared.NotImplemented("remote.RevokeAccess")
}

// SearchPrincipals searches people and teams by name, email or team.
func (s *Service) SearchPrincipals(ctx context.Context, query string) ([]shared.Principal, error) {
	return nil, shared.NotImplemented("remote.SearchPrincipals")
}
