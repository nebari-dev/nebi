package auth

import (
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/rbac"
)

var _ rbac.Provider = (*stubRBACProvider)(nil)

type stubRBACProvider struct {
	isAdmin                bool
	isAdminErr             error
	makeAdminErr           error
	revokeAdminErr         error
	addUserToGroupErr      error
	removeUserFromGroupErr error
	getUserGroupsErr       error

	madeAdmin     bool
	revokedAdmin  bool
	addedGroups   []uuid.UUID
	removedGroups []uuid.UUID
	userGroups    []uuid.UUID
}

func (p *stubRBACProvider) CanReadWorkspace(_, _ uuid.UUID) (bool, error) {
	return true, nil
}

func (p *stubRBACProvider) CanWriteWorkspace(_, _ uuid.UUID) (bool, error) {
	return true, nil
}

func (p *stubRBACProvider) CanReadRegistry(_, _ uuid.UUID) (bool, error) {
	return true, nil
}

func (p *stubRBACProvider) CanWriteRegistry(_, _ uuid.UUID) (bool, error) {
	return true, nil
}

func (p *stubRBACProvider) IsAdmin(uuid.UUID) (bool, error) {
	return p.isAdmin, p.isAdminErr
}

func (p *stubRBACProvider) GrantWorkspaceAccess(_, _ uuid.UUID, _ string) error {
	return nil
}

func (p *stubRBACProvider) RevokeWorkspaceAccess(_, _ uuid.UUID) error {
	return nil
}

func (p *stubRBACProvider) MakeAdmin(uuid.UUID) error {
	p.madeAdmin = true
	return p.makeAdminErr
}

func (p *stubRBACProvider) RevokeAdmin(uuid.UUID) error {
	p.revokedAdmin = true
	return p.revokeAdminErr
}

func (p *stubRBACProvider) GetAllAdminUserIDs() (map[uuid.UUID]bool, error) {
	return map[uuid.UUID]bool{}, nil
}

func (p *stubRBACProvider) AddUserToGroup(_, groupID uuid.UUID) error {
	p.addedGroups = append(p.addedGroups, groupID)
	return p.addUserToGroupErr
}

func (p *stubRBACProvider) RemoveUserFromGroup(_, groupID uuid.UUID) error {
	p.removedGroups = append(p.removedGroups, groupID)
	return p.removeUserFromGroupErr
}

func (p *stubRBACProvider) GetUserGroups(uuid.UUID) ([]uuid.UUID, error) {
	return p.userGroups, p.getUserGroupsErr
}

func (p *stubRBACProvider) GrantGroupWorkspaceAccess(_, _ uuid.UUID, _ string) error {
	return nil
}

func (p *stubRBACProvider) RevokeGroupWorkspaceAccess(_, _ uuid.UUID) error {
	return nil
}

func (p *stubRBACProvider) GrantGroupRegistryAccess(_, _ uuid.UUID, _ string) error {
	return nil
}

func (p *stubRBACProvider) RevokeGroupRegistryAccess(_, _ uuid.UUID) error {
	return nil
}

func (p *stubRBACProvider) MakeGroupAdmin(uuid.UUID) error {
	return nil
}

func (p *stubRBACProvider) RevokeGroupAdmin(uuid.UUID) error {
	return nil
}

func (p *stubRBACProvider) RemoveAllGroupPolicies(uuid.UUID) error {
	return nil
}
