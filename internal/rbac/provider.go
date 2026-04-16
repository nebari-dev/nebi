package rbac

import "github.com/google/uuid"

// Provider abstracts RBAC operations so callers can use dependency injection
// instead of the global enforcer. This enables per-test isolation and mocking.
type Provider interface {
	CanReadWorkspace(userID, wsID uuid.UUID) (bool, error)
	CanWriteWorkspace(userID, wsID uuid.UUID) (bool, error)
	IsAdmin(userID uuid.UUID) (bool, error)
	GrantWorkspaceAccess(userID, wsID uuid.UUID, role string) error
	RevokeWorkspaceAccess(userID, wsID uuid.UUID) error
	MakeAdmin(userID uuid.UUID) error
	RevokeAdmin(userID uuid.UUID) error
	GetAllAdminUserIDs() (map[uuid.UUID]bool, error)
}

// DefaultProvider wraps the global Casbin enforcer as an rbac.Provider.
type DefaultProvider struct{}

func NewDefaultProvider() *DefaultProvider { return &DefaultProvider{} }

func (DefaultProvider) CanReadWorkspace(userID, wsID uuid.UUID) (bool, error) {
	return CanReadWorkspace(userID, wsID)
}
func (DefaultProvider) CanWriteWorkspace(userID, wsID uuid.UUID) (bool, error) {
	return CanWriteWorkspace(userID, wsID)
}
func (DefaultProvider) IsAdmin(userID uuid.UUID) (bool, error) {
	return IsAdmin(userID)
}
func (DefaultProvider) GrantWorkspaceAccess(userID, wsID uuid.UUID, role string) error {
	return GrantWorkspaceAccess(userID, wsID, role)
}
func (DefaultProvider) RevokeWorkspaceAccess(userID, wsID uuid.UUID) error {
	return RevokeWorkspaceAccess(userID, wsID)
}
func (DefaultProvider) MakeAdmin(userID uuid.UUID) error {
	return MakeAdmin(userID)
}
func (DefaultProvider) RevokeAdmin(userID uuid.UUID) error {
	return RevokeAdmin(userID)
}
func (DefaultProvider) GetAllAdminUserIDs() (map[uuid.UUID]bool, error) {
	return GetAllAdminUserIDs()
}
