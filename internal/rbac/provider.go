package rbac

import "github.com/google/uuid"

// Provider abstracts RBAC operations so callers can use dependency injection
// instead of the global enforcer. This enables per-test isolation and mocking.
type Provider interface {
	CanReadProject(userID, projectID uuid.UUID) (bool, error)
	CanWriteProject(userID, projectID uuid.UUID) (bool, error)
	CanReadRegistry(userID, regID uuid.UUID) (bool, error)
	CanWriteRegistry(userID, regID uuid.UUID) (bool, error)
	IsAdmin(userID uuid.UUID) (bool, error)
	GrantProjectAccess(userID, projectID uuid.UUID, role string) error
	RevokeProjectAccess(userID, projectID uuid.UUID) error
	MakeAdmin(userID uuid.UUID) error
	RevokeAdmin(userID uuid.UUID) error
	GetAllAdminUserIDs() (map[uuid.UUID]bool, error)

	// Group operations
	AddUserToGroup(userID, groupID uuid.UUID) error
	RemoveUserFromGroup(userID, groupID uuid.UUID) error
	GetUserGroups(userID uuid.UUID) ([]uuid.UUID, error)
	GrantGroupProjectAccess(groupID, projectID uuid.UUID, role string) error
	RevokeGroupProjectAccess(groupID, projectID uuid.UUID) error
	GrantGroupRegistryAccess(groupID, regID uuid.UUID, action string) error
	RevokeGroupRegistryAccess(groupID, regID uuid.UUID) error
	MakeGroupAdmin(groupID uuid.UUID) error
	RevokeGroupAdmin(groupID uuid.UUID) error
	RemoveAllGroupPolicies(groupID uuid.UUID) error
}

// DefaultProvider wraps the global Casbin enforcer as an rbac.Provider.
type DefaultProvider struct{}

func NewDefaultProvider() *DefaultProvider { return &DefaultProvider{} }

func (DefaultProvider) CanReadProject(userID, projectID uuid.UUID) (bool, error) {
	return CanReadProject(userID, projectID)
}
func (DefaultProvider) CanWriteProject(userID, projectID uuid.UUID) (bool, error) {
	return CanWriteProject(userID, projectID)
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
func (DefaultProvider) GrantProjectAccess(userID, projectID uuid.UUID, role string) error {
	return GrantProjectAccess(userID, projectID, role)
}
func (DefaultProvider) RevokeProjectAccess(userID, projectID uuid.UUID) error {
	return RevokeProjectAccess(userID, projectID)
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
func (DefaultProvider) GrantGroupProjectAccess(groupID, projectID uuid.UUID, role string) error {
	return GrantGroupProjectAccess(groupID, projectID, role)
}
func (DefaultProvider) RevokeGroupProjectAccess(groupID, projectID uuid.UUID) error {
	return RevokeGroupProjectAccess(groupID, projectID)
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
