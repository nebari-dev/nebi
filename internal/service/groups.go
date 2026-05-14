package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// GroupService owns CRUD + membership ops for groups. Workspace shares and
// registry/admin grants for groups live in WorkspaceService and AdminService
// to keep their existing locality of behaviour.
type GroupService struct {
	db   *gorm.DB
	rbac rbac.Provider
}

func NewGroupService(db *gorm.DB, rbacProvider rbac.Provider) *GroupService {
	return &GroupService{db: db, rbac: rbacProvider}
}

// CreateGroupRequest is the input for CreateGroup.
type CreateGroupRequest struct {
	Name        string
	Description string
}

// UpdateGroupRequest holds optional fields for PATCH.
type UpdateGroupRequest struct {
	Name        *string
	Description *string
}

// GroupWithMemberCount denormalises member_count for list views.
type GroupWithMemberCount struct {
	models.Group
	MemberCount int64 `json:"member_count"`
}

// CreateGroup creates a native group. Admin-only; the handler is responsible
// for the admin check.
func (s *GroupService) CreateGroup(req CreateGroupRequest, actorID uuid.UUID) (*models.Group, error) {
	if req.Name == "" {
		return nil, &ValidationError{Message: "Group name is required"}
	}

	var existing models.Group
	if err := s.db.Where("name = ?", req.Name).First(&existing).Error; err == nil {
		return nil, &ConflictError{Message: "Group with this name already exists"}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	g := models.Group{
		Name:        req.Name,
		Description: req.Description,
		Source:      models.GroupSourceNative,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&g).Error; err != nil {
			return fmt.Errorf("create group: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionCreateGroup, fmt.Sprintf("group:%s", g.ID), map[string]interface{}{
			"name": g.Name,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ListGroups returns every group with denormalised member count.
func (s *GroupService) ListGroups() ([]GroupWithMemberCount, error) {
	var groups []models.Group
	if err := s.db.Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	out := make([]GroupWithMemberCount, len(groups))
	for i, g := range groups {
		var count int64
		s.db.Model(&models.GroupMember{}).Where("group_id = ?", g.ID).Count(&count)
		out[i] = GroupWithMemberCount{Group: g, MemberCount: count}
	}
	return out, nil
}

// GetGroup returns one group + member count.
func (s *GroupService) GetGroup(id uuid.UUID) (*GroupWithMemberCount, error) {
	var g models.Group
	if err := s.db.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var count int64
	s.db.Model(&models.GroupMember{}).Where("group_id = ?", g.ID).Count(&count)
	return &GroupWithMemberCount{Group: g, MemberCount: count}, nil
}

// UpdateGroup updates name/description on a native group. OIDC groups return ConflictError.
func (s *GroupService) UpdateGroup(id uuid.UUID, req UpdateGroupRequest, actorID uuid.UUID) (*models.Group, error) {
	var g models.Group
	if err := s.db.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if g.Source == models.GroupSourceOIDC {
		return nil, &ConflictError{Message: "OIDC-synced groups are read-only"}
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) == 0 {
		return &g, nil
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&g).Updates(updates).Error; err != nil {
			return fmt.Errorf("update group: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionUpdateGroup, fmt.Sprintf("group:%s", g.ID), updates)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// DeleteGroup soft-deletes the group row (GORM) and hard-removes every Casbin
// policy + grouping rule that referenced it (Casbin does not honour
// gorm.DeletedAt). OIDC groups return ConflictError — they are reconciled by
// login flow only.
func (s *GroupService) DeleteGroup(id uuid.UUID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if g.Source == models.GroupSourceOIDC {
		return &ConflictError{Message: "OIDC-synced groups are read-only"}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", g.ID).Delete(&models.GroupMember{}).Error; err != nil {
			return fmt.Errorf("delete members: %w", err)
		}
		if err := tx.Unscoped().Where("group_id = ?", g.ID).Delete(&models.GroupPermission{}).Error; err != nil {
			return fmt.Errorf("delete group permissions: %w", err)
		}
		// Hard-delete the group row so its name is freed for re-creation (the
		// unique index on name doesn't honour gorm.DeletedAt — a soft-deleted
		// row blocks any future create with the same name, including OIDC sync
		// re-creating a group after the admin deletes the native version).
		if err := tx.Unscoped().Delete(&g).Error; err != nil {
			return fmt.Errorf("delete group: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionDeleteGroup, fmt.Sprintf("group:%s", g.ID), map[string]interface{}{"name": g.Name})
		return nil
	})
	if err != nil {
		return err
	}

	if err := s.rbac.RemoveAllGroupPolicies(g.ID); err != nil {
		return fmt.Errorf("remove casbin policies: %w", err)
	}
	return nil
}

// ListMembers returns every user in a group with denormalised user fields.
func (s *GroupService) ListMembers(groupID uuid.UUID) ([]models.GroupMember, error) {
	var members []models.GroupMember
	if err := s.db.Preload("User").Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

// AddMember adds a user to a group. Idempotent: existing membership is a no-op.
func (s *GroupService) AddMember(groupID, userID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if g.Source == models.GroupSourceOIDC {
		return &ConflictError{Message: "OIDC-synced groups manage members from the IdP"}
	}

	var existing models.GroupMember
	err := s.db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	var u models.User
	if err := s.db.First(&u, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ValidationError{Message: "User not found"}
		}
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		m := models.GroupMember{GroupID: groupID, UserID: userID}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("create member: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionAddGroupMember, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
			"user_id": userID,
		})
		return nil
	})
	if err != nil {
		return err
	}

	return s.rbac.AddUserToGroup(userID, groupID)
}

// RemoveMember removes a user from a group.
func (s *GroupService) RemoveMember(groupID, userID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if g.Source == models.GroupSourceOIDC {
		return &ConflictError{Message: "OIDC-synced groups manage members from the IdP"}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&models.GroupMember{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		audit.LogAction(tx, actorID, audit.ActionRemoveGroupMember, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
			"user_id": userID,
		})
		return nil
	})
	if err != nil {
		return err
	}

	return s.rbac.RemoveUserFromGroup(userID, groupID)
}

// ListGroupsForUser returns groups the user belongs to (used by /groups/me).
func (s *GroupService) ListGroupsForUser(userID uuid.UUID) ([]models.Group, error) {
	var groups []models.Group
	err := s.db.
		Joins("JOIN group_members gm ON gm.group_id = groups.id").
		Where("gm.user_id = ?", userID).
		Find(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	return groups, nil
}
