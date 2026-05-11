package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// ShareWorkspace grants a user access to a workspace. Only the owner can share.
func (s *WorkspaceService) ShareWorkspace(wsID string, ownerID uuid.UUID, targetUserID uuid.UUID, role string) (*models.Permission, error) {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid workspace ID"}
	}

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if ws.OwnerID != ownerID {
		return nil, &ForbiddenError{Message: "Only the owner can share this workspace"}
	}

	// Verify target user exists
	var targetUser models.User
	if err := s.db.First(&targetUser, "id = ?", targetUserID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ValidationError{Message: "User not found"}
		}
		return nil, err
	}

	if role != "viewer" && role != "editor" {
		return nil, &ValidationError{Message: "Role must be 'viewer' or 'editor'"}
	}

	// Get role record
	var roleRecord models.Role
	if err := s.db.Where("name = ?", role).First(&roleRecord).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	var permission models.Permission
	err = s.db.Transaction(func(tx *gorm.DB) error {
		permission = models.Permission{
			UserID:      targetUserID,
			WorkspaceID: wsUUID,
			RoleID:      roleRecord.ID,
		}
		if err := tx.Create(&permission).Error; err != nil {
			return fmt.Errorf("create permission: %w", err)
		}

		audit.LogAction(tx, ownerID, audit.ActionGrantPermission, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
			"target_user_id": targetUserID,
			"role":           role,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	// RBAC outside transaction — Casbin uses its own DB connection
	if err := s.rbac.GrantWorkspaceAccess(targetUserID, wsUUID, role); err != nil {
		return nil, fmt.Errorf("grant RBAC permission: %w", err)
	}

	return &permission, nil
}

// UnshareWorkspace revokes a user's access to a workspace. Only the owner can unshare.
func (s *WorkspaceService) UnshareWorkspace(wsID string, ownerID uuid.UUID, targetUserID uuid.UUID) error {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return &ValidationError{Message: "Invalid workspace ID"}
	}

	targetUUID := targetUserID

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	if ws.OwnerID != ownerID {
		return &ForbiddenError{Message: "Only the owner can unshare this workspace"}
	}

	if targetUUID == ownerID {
		return &ValidationError{Message: "Cannot remove owner's access"}
	}

	// Find permission
	var permission models.Permission
	if err := s.db.Where("user_id = ? AND workspace_id = ?", targetUUID, wsUUID).First(&permission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&permission).Error; err != nil {
			return fmt.Errorf("delete permission: %w", err)
		}

		audit.LogAction(tx, ownerID, audit.ActionRevokePermission, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
			"target_user_id": targetUUID,
		})

		return nil
	})
	if err != nil {
		return err
	}

	// RBAC outside transaction — Casbin uses its own DB connection
	if err := s.rbac.RevokeWorkspaceAccess(targetUUID, wsUUID); err != nil {
		return fmt.Errorf("revoke RBAC permission: %w", err)
	}

	return nil
}

// ListCollaborators returns every user (Kind=user) and group (Kind=group) with
// access to the workspace, plus the owner. Groups are tagged with their source.
func (s *WorkspaceService) ListCollaborators(wsID string) ([]CollaboratorResult, error) {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid workspace ID"}
	}

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	collaborators := []CollaboratorResult{}

	// Owner.
	var owner models.User
	if err := s.db.First(&owner, "id = ?", ws.OwnerID).Error; err == nil {
		collaborators = append(collaborators, CollaboratorResult{
			Kind:     CollaboratorKindUser,
			UserID:   ws.OwnerID,
			Username: owner.Username,
			Email:    owner.Email,
			Role:     "owner",
			IsOwner:  true,
		})
	}

	// Per-user permissions.
	var userPerms []models.Permission
	if err := s.db.Preload("User").Preload("Role").Where("workspace_id = ?", wsUUID).Find(&userPerms).Error; err != nil {
		return nil, fmt.Errorf("fetch user collaborators: %w", err)
	}
	for _, p := range userPerms {
		if p.UserID == ws.OwnerID {
			continue
		}
		collaborators = append(collaborators, CollaboratorResult{
			Kind:     CollaboratorKindUser,
			UserID:   p.UserID,
			Username: p.User.Username,
			Email:    p.User.Email,
			Role:     p.Role.Name,
			IsOwner:  false,
		})
	}

	// Per-group permissions.
	var groupPerms []models.GroupPermission
	if err := s.db.Preload("Group").Preload("Role").Where("workspace_id = ?", wsUUID).Find(&groupPerms).Error; err != nil {
		return nil, fmt.Errorf("fetch group collaborators: %w", err)
	}
	for _, gp := range groupPerms {
		collaborators = append(collaborators, CollaboratorResult{
			Kind:    CollaboratorKindGroup,
			GroupID: gp.GroupID,
			Name:    gp.Group.Name,
			Source:  string(gp.Group.Source),
			Role:    gp.Role.Name,
			IsOwner: false,
		})
	}

	return collaborators, nil
}

// ShareWorkspaceWithGroup grants a group access to a workspace.
// Authorization: caller must be admin OR (owner AND member of the group).
// The handler performs the admin check; the service performs the
// owner+member check (which it can do without re-deriving admin state).
//
// `groupSvc` is injected so we can query membership without importing the
// group service at type-level (which would create a cycle).
func (s *WorkspaceService) ShareWorkspaceWithGroup(wsID string, callerID uuid.UUID, groupID uuid.UUID, role string, groupSvc *GroupService) (*models.GroupPermission, error) {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid workspace ID"}
	}

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ValidationError{Message: "Group not found"}
		}
		return nil, err
	}

	// Admin bypass: if caller is admin, allow regardless of membership.
	isAdmin, err := s.rbac.IsAdmin(callerID)
	if err != nil {
		return nil, fmt.Errorf("check admin: %w", err)
	}
	if !isAdmin {
		if ws.OwnerID != callerID {
			return nil, &ForbiddenError{Message: "Only the owner or an admin can share this workspace"}
		}
		// Owner must be a member of the group they're sharing to.
		userGroups, err := s.rbac.GetUserGroups(callerID)
		if err != nil {
			return nil, fmt.Errorf("check group membership: %w", err)
		}
		var member bool
		for _, gid := range userGroups {
			if gid == groupID {
				member = true
				break
			}
		}
		if !member {
			return nil, &ForbiddenError{Message: "You can only share with groups you belong to"}
		}
	}

	if role != "viewer" && role != "editor" {
		return nil, &ValidationError{Message: "Role must be 'viewer' or 'editor'"}
	}
	var roleRecord models.Role
	if err := s.db.Where("name = ?", role).First(&roleRecord).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	// Reject duplicate.
	var existing models.GroupPermission
	if err := s.db.Where("group_id = ? AND workspace_id = ?", groupID, wsUUID).First(&existing).Error; err == nil {
		return nil, &ConflictError{Message: "Group already has permission on this workspace"}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var permission models.GroupPermission
	err = s.db.Transaction(func(tx *gorm.DB) error {
		permission = models.GroupPermission{
			GroupID:     groupID,
			WorkspaceID: wsUUID,
			RoleID:      roleRecord.ID,
		}
		if err := tx.Create(&permission).Error; err != nil {
			return fmt.Errorf("create group permission: %w", err)
		}
		audit.LogAction(tx, callerID, audit.ActionGrantGroupPerm, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
			"group_id": groupID,
			"role":     role,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.rbac.GrantGroupWorkspaceAccess(groupID, wsUUID, role); err != nil {
		return nil, fmt.Errorf("grant RBAC: %w", err)
	}
	return &permission, nil
}

// UnshareWorkspaceFromGroup revokes a group's access. Same auth rules as ShareWorkspaceWithGroup.
func (s *WorkspaceService) UnshareWorkspaceFromGroup(wsID string, callerID uuid.UUID, groupID uuid.UUID) error {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return &ValidationError{Message: "Invalid workspace ID"}
	}

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	isAdmin, err := s.rbac.IsAdmin(callerID)
	if err != nil {
		return fmt.Errorf("check admin: %w", err)
	}
	if !isAdmin && ws.OwnerID != callerID {
		return &ForbiddenError{Message: "Only the owner or an admin can unshare this workspace"}
	}

	var permission models.GroupPermission
	if err := s.db.Where("group_id = ? AND workspace_id = ?", groupID, wsUUID).First(&permission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&permission).Error; err != nil {
			return fmt.Errorf("delete group permission: %w", err)
		}
		audit.LogAction(tx, callerID, audit.ActionRevokeGroupPerm, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
			"group_id": groupID,
		})
		return nil
	})
	if err != nil {
		return err
	}

	return s.rbac.RevokeGroupWorkspaceAccess(groupID, wsUUID)
}
