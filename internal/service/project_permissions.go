package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// ShareProject grants a user access to a project. Only the owner can share.
func (s *ProjectService) ShareProject(projectID string, ownerID uuid.UUID, targetUserID uuid.UUID, role string) (*models.Permission, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid project ID"}
	}

	var project models.Project
	if err := s.db.Where("id = ?", projectUUID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if project.OwnerID != ownerID {
		return nil, &ForbiddenError{Message: "Only the owner can share this project"}
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
			UserID:    targetUserID,
			ProjectID: projectUUID,
			RoleID:    roleRecord.ID,
		}
		if err := tx.Create(&permission).Error; err != nil {
			return fmt.Errorf("create permission: %w", err)
		}

		audit.LogAction(tx, ownerID, audit.ActionGrantPermission, fmt.Sprintf("project:%s", projectUUID.String()), map[string]interface{}{
			"target_user_id": targetUserID,
			"role":           role,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	// RBAC outside transaction — Casbin uses its own DB connection
	if err := s.rbac.GrantProjectAccess(targetUserID, projectUUID, role); err != nil {
		return nil, fmt.Errorf("grant RBAC permission: %w", err)
	}

	return &permission, nil
}

// UnshareProject revokes a user's access to a project. Only the owner can unshare.
func (s *ProjectService) UnshareProject(projectID string, ownerID uuid.UUID, targetUserID uuid.UUID) error {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return &ValidationError{Message: "Invalid project ID"}
	}

	targetUUID := targetUserID

	var project models.Project
	if err := s.db.Where("id = ?", projectUUID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	if project.OwnerID != ownerID {
		return &ForbiddenError{Message: "Only the owner can unshare this project"}
	}

	if targetUUID == ownerID {
		return &ValidationError{Message: "Cannot remove owner's access"}
	}

	// Find permission
	var permission models.Permission
	if err := s.db.Where("user_id = ? AND project_id = ?", targetUUID, projectUUID).First(&permission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&permission).Error; err != nil {
			return fmt.Errorf("delete permission: %w", err)
		}

		audit.LogAction(tx, ownerID, audit.ActionRevokePermission, fmt.Sprintf("project:%s", projectUUID.String()), map[string]interface{}{
			"target_user_id": targetUUID,
		})

		return nil
	})
	if err != nil {
		return err
	}

	// RBAC outside transaction — Casbin uses its own DB connection
	if err := s.rbac.RevokeProjectAccess(targetUUID, projectUUID); err != nil {
		return fmt.Errorf("revoke RBAC permission: %w", err)
	}

	return nil
}

// ListCollaborators returns every user (Kind=user) and group (Kind=group) with
// access to the project, plus the owner. Groups are tagged with their source.
func (s *ProjectService) ListCollaborators(projectID string) ([]CollaboratorResult, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid project ID"}
	}

	var project models.Project
	if err := s.db.Where("id = ?", projectUUID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	collaborators := []CollaboratorResult{}

	// Owner.
	var owner models.User
	if err := s.db.First(&owner, "id = ?", project.OwnerID).Error; err == nil {
		collaborators = append(collaborators, CollaboratorResult{
			Kind:     CollaboratorKindUser,
			UserID:   &project.OwnerID,
			Username: owner.Username,
			Email:    owner.Email,
			Role:     "owner",
			IsOwner:  true,
		})
	}

	// Per-user permissions.
	var userPerms []models.Permission
	if err := s.db.Preload("User").Preload("Role").Where("project_id = ?", projectUUID).Find(&userPerms).Error; err != nil {
		return nil, fmt.Errorf("fetch user collaborators: %w", err)
	}
	for _, p := range userPerms {
		if p.UserID == project.OwnerID {
			continue
		}
		collaborators = append(collaborators, CollaboratorResult{
			Kind:     CollaboratorKindUser,
			UserID:   &p.UserID,
			Username: p.User.Username,
			Email:    p.User.Email,
			Role:     p.Role.Name,
			IsOwner:  false,
		})
	}

	// Per-group permissions.
	var groupPerms []models.GroupPermission
	if err := s.db.Preload("Group").Preload("Role").Where("project_id = ?", projectUUID).Find(&groupPerms).Error; err != nil {
		return nil, fmt.Errorf("fetch group collaborators: %w", err)
	}
	for _, gp := range groupPerms {
		collaborators = append(collaborators, CollaboratorResult{
			Kind:    CollaboratorKindGroup,
			GroupID: &gp.GroupID,
			Name:    gp.Group.Name,
			Source:  string(gp.Group.Source),
			Role:    gp.Role.Name,
			IsOwner: false,
		})
	}

	return collaborators, nil
}

// ShareProjectWithGroup grants a group access to a project.
// Authorization: caller must be admin OR (owner AND member of the group).
// Membership is resolved via Casbin grouping rules, so the GroupService
// dependency is not needed here.
func (s *ProjectService) ShareProjectWithGroup(projectID string, callerID uuid.UUID, groupID uuid.UUID, role string) (*models.GroupPermission, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid project ID"}
	}

	var project models.Project
	if err := s.db.Where("id = ?", projectUUID).First(&project).Error; err != nil {
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
		if project.OwnerID != callerID {
			return nil, &ForbiddenError{Message: "Only the owner or an admin can share this project"}
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
	if err := s.db.Where("group_id = ? AND project_id = ?", groupID, projectUUID).First(&existing).Error; err == nil {
		return nil, &ConflictError{Message: "Group already has permission on this project"}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var permission models.GroupPermission
	err = s.db.Transaction(func(tx *gorm.DB) error {
		permission = models.GroupPermission{
			GroupID:   groupID,
			ProjectID: projectUUID,
			RoleID:    roleRecord.ID,
		}
		if err := tx.Create(&permission).Error; err != nil {
			return fmt.Errorf("create group permission: %w", err)
		}
		audit.LogAction(tx, callerID, audit.ActionGrantGroupPerm, fmt.Sprintf("project:%s", projectUUID.String()), map[string]interface{}{
			"group_id": groupID,
			"role":     role,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.rbac.GrantGroupProjectAccess(groupID, projectUUID, role); err != nil {
		return nil, fmt.Errorf("grant RBAC: %w", err)
	}
	return &permission, nil
}

// UnshareProjectFromGroup revokes a group's access. Same auth rules as ShareProjectWithGroup.
func (s *ProjectService) UnshareProjectFromGroup(projectID string, callerID uuid.UUID, groupID uuid.UUID) error {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return &ValidationError{Message: "Invalid project ID"}
	}

	var project models.Project
	if err := s.db.Where("id = ?", projectUUID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	isAdmin, err := s.rbac.IsAdmin(callerID)
	if err != nil {
		return fmt.Errorf("check admin: %w", err)
	}
	if !isAdmin && project.OwnerID != callerID {
		return &ForbiddenError{Message: "Only the owner or an admin can unshare this project"}
	}

	var permission models.GroupPermission
	if err := s.db.Where("group_id = ? AND project_id = ?", groupID, projectUUID).First(&permission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&permission).Error; err != nil {
			return fmt.Errorf("delete group permission: %w", err)
		}
		audit.LogAction(tx, callerID, audit.ActionRevokeGroupPerm, fmt.Sprintf("project:%s", projectUUID.String()), map[string]interface{}{
			"group_id": groupID,
		})
		return nil
	})
	if err != nil {
		return err
	}

	return s.rbac.RevokeGroupProjectAccess(groupID, projectUUID)
}
