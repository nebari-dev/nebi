package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// GroupService reads groups. Groups and their memberships are owned by the
// identity provider and synced on authentication (see internal/auth), so
// there are no write operations here. Project shares and registry grants for
// groups live in ProjectService and AdminService.
type GroupService struct {
	db *gorm.DB
}

func NewGroupService(db *gorm.DB) *GroupService {
	return &GroupService{db: db}
}

// GroupWithMemberCount denormalises member_count for list views.
type GroupWithMemberCount struct {
	models.Group
	MemberCount int64 `json:"member_count"`
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

// ListMembers returns every user in a group with denormalised user fields.
func (s *GroupService) ListMembers(groupID uuid.UUID) ([]models.GroupMember, error) {
	var members []models.GroupMember
	if err := s.db.Preload("User").Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
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
