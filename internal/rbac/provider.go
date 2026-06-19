package rbac

import "github.com/google/uuid"

// Provider abstracts RBAC operations so callers can use dependency injection
// instead of the global enforcer. This enables per-test isolation and mocking.
type Provider interface {
	CanReadWorkspace(userID, wsID uuid.UUID) (bool, error)
	CanWriteWorkspace(userID, wsID uuid.UUID) (bool, error)
	CanReadRegistry(userID, regID uuid.UUID) (bool, error)
	CanWriteRegistry(userID, regID uuid.UUID) (bool, error)
	IsAdmin(userID uuid.UUID) (bool, error)
	GrantWorkspaceAccess(userID, wsID uuid.UUID, role string) error
	RevokeWorkspaceAccess(userID, wsID uuid.UUID) error
	MakeAdmin(userID uuid.UUID) error
	RevokeAdmin(userID uuid.UUID) error
	GetAllAdminUserIDs() (map[uuid.UUID]bool, error)

	// Group operations
	AddUserToGroup(userID, groupID uuid.UUID) error
	RemoveUserFromGroup(userID, groupID uuid.UUID) error
	GetUserGroups(userID uuid.UUID) ([]uuid.UUID, error)
	GrantGroupWorkspaceAccess(groupID, wsID uuid.UUID, role string) error
	RevokeGroupWorkspaceAccess(groupID, wsID uuid.UUID) error
	GrantGroupRegistryAccess(groupID, regID uuid.UUID, action string) error
	RevokeGroupRegistryAccess(groupID, regID uuid.UUID) error
	MakeGroupAdmin(groupID uuid.UUID) error
	RevokeGroupAdmin(groupID uuid.UUID) error
	RemoveAllGroupPolicies(groupID uuid.UUID) error
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
func (DefaultProvider) CanReadRegistry(userID, regID uuid.UUID) (bool, error) {
	return CanReadRegistry(userID, regID)
}
func (DefaultProvider) CanWriteRegistry(userID, regID uuid.UUID) (bool, error) {
	return CanWriteRegistry(userID, regID)
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
func (DefaultProvider) AddUserToGroup(userID, groupID uuid.UUID) error {
	return AddUserToGroup(userID, groupID)
}
func (DefaultProvider) RemoveUserFromGroup(userID, groupID uuid.UUID) error {
	return RemoveUserFromGroup(userID, groupID)
}
func (DefaultProvider) GetUserGroups(userID uuid.UUID) ([]uuid.UUID, error) {
	return GetUserGroups(userID)
}
func (DefaultProvider) GrantGroupWorkspaceAccess(groupID, wsID uuid.UUID, role string) error {
	return GrantGroupWorkspaceAccess(groupID, wsID, role)
}
func (DefaultProvider) RevokeGroupWorkspaceAccess(groupID, wsID uuid.UUID) error {
	return RevokeGroupWorkspaceAccess(groupID, wsID)
}
func (DefaultProvider) GrantGroupRegistryAccess(groupID, regID uuid.UUID, action string) error {
	return GrantGroupRegistryAccess(groupID, regID, action)
}
func (DefaultProvider) RevokeGroupRegistryAccess(groupID, regID uuid.UUID) error {
	return RevokeGroupRegistryAccess(groupID, regID)
}
func (DefaultProvider) MakeGroupAdmin(groupID uuid.UUID) error {
	return MakeGroupAdmin(groupID)
}
func (DefaultProvider) RevokeGroupAdmin(groupID uuid.UUID) error {
	return RevokeGroupAdmin(groupID)
}
func (DefaultProvider) RemoveAllGroupPolicies(groupID uuid.UUID) error {
	return RemoveAllGroupPolicies(groupID)
}
