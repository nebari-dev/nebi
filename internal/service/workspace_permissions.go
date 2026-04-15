package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
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

	// Create permission record
	permission := models.Permission{
		UserID:      targetUserID,
		WorkspaceID: wsUUID,
		RoleID:      roleRecord.ID,
	}
	if err := s.db.Create(&permission).Error; err != nil {
		return nil, fmt.Errorf("create permission: %w", err)
	}

	// Grant in RBAC
	if err := rbac.GrantWorkspaceAccess(targetUserID, wsUUID, role); err != nil {
		return nil, fmt.Errorf("grant RBAC permission: %w", err)
	}

	audit.LogAction(s.db, ownerID, audit.ActionGrantPermission, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
		"target_user_id": targetUserID,
		"role":           role,
	})

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

	// Revoke from RBAC
	if err := rbac.RevokeWorkspaceAccess(targetUUID, wsUUID); err != nil {
		return fmt.Errorf("revoke RBAC permission: %w", err)
	}

	// Delete permission record
	if err := s.db.Delete(&permission).Error; err != nil {
		return fmt.Errorf("delete permission: %w", err)
	}

	audit.LogAction(s.db, ownerID, audit.ActionRevokePermission, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
		"target_user_id": targetUUID,
	})

	return nil
}

// ListCollaborators returns all users with access to a workspace (owner + shared users).
func (s *WorkspaceService) ListCollaborators(wsID string) ([]CollaboratorResult, error) {
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

	// Get all permissions for this workspace
	var permissions []models.Permission
	if err := s.db.Preload("User").Preload("Role").Where("workspace_id = ?", wsUUID).Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("fetch collaborators: %w", err)
	}

	// Start with owner
	var owner models.User
	collaborators := []CollaboratorResult{}

	if err := s.db.First(&owner, "id = ?", ws.OwnerID).Error; err == nil {
		collaborators = append(collaborators, CollaboratorResult{
			UserID:   ws.OwnerID,
			Username: owner.Username,
			Email:    owner.Email,
			Role:     "owner",
			IsOwner:  true,
		})
	}

	// Add other collaborators (excluding owner if they have a permission record)
	for _, perm := range permissions {
		if perm.UserID != ws.OwnerID {
			collaborators = append(collaborators, CollaboratorResult{
				UserID:   perm.UserID,
				Username: perm.User.Username,
				Email:    perm.User.Email,
				Role:     perm.Role.Name,
				IsOwner:  false,
			})
		}
	}

	return collaborators, nil
}
