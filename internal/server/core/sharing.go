package core

// Owner: U6 (sharing). Roles map onto the existing RBAC roles:
// owner -> project owner, edit -> "editor", view -> "viewer".

import (
	"context"

	"github.com/google/uuid"
)

// SearchPrincipals finds users (username, email) and groups (name).
func (s *Service) SearchPrincipals(ctx context.Context, query string) ([]Principal, error) {
	return nil, notImplemented("core.SearchPrincipals")
}

// ListAccess returns who has access; CanManage is true for owners/admins.
func (s *Service) ListAccess(ctx context.Context, callerID, projectID uuid.UUID) (*AccessList, error) {
	return nil, notImplemented("core.ListAccess")
}

// GrantAccess adds principals with a role (owners/admins only).
func (s *Service) GrantAccess(ctx context.Context, callerID, projectID uuid.UUID, req GrantAccessRequest) (*AccessList, error) {
	return nil, notImplemented("core.GrantAccess")
}

// UpdateAccess changes a principal's role (owners/admins only).
func (s *Service) UpdateAccess(ctx context.Context, callerID, projectID uuid.UUID, p PrincipalRef, req UpdateAccessRequest) (*AccessList, error) {
	return nil, notImplemented("core.UpdateAccess")
}

// RevokeAccess removes a principal (owners/admins only).
func (s *Service) RevokeAccess(ctx context.Context, callerID, projectID uuid.UUID, p PrincipalRef) (*AccessList, error) {
	return nil, notImplemented("core.RevokeAccess")
}
