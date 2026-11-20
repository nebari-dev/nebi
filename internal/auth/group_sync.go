package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// syncOIDCGroups reconciles the user's group memberships with the names in
// the token's `groups` claim. Groups are owned by the identity provider:
// unknown names are created, and memberships missing from the claim are
// removed. Idempotent: safe to call for every new token. Zero-member groups
// are preserved so existing project shares survive churn. The RBAC provider
// is injected so tests can fail known reconciliation steps.
func syncOIDCGroups(db *gorm.DB, userID uuid.UUID, claimGroups []string, rbacProvider rbac.Provider) error {
	if err := validateOIDCGroupSyncInputs(db, rbacProvider); err != nil {
		return err
	}

	desiredNames, desired := normalizedOIDCGroupSet(claimGroups)
	if err := syncOIDCGroupRemovalsWithDesired(db, userID, desired, rbacProvider); err != nil {
		return err
	}

	desiredGroupIDs := make([]uuid.UUID, 0, len(desired))

	if err := db.Transaction(func(tx *gorm.DB) error {
		for name := range desired {
			g, err := findOrCreateOIDCGroup(tx, name, userID)
			if err != nil {
				return err
			}

			// Concurrent syncs for the same user (e.g. parallel requests with a
			// fresh token) may race on the same membership, so tolerate an
			// existing row instead of failing.
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&models.GroupMember{GroupID: g.ID, UserID: userID})
			if result.Error != nil {
				return fmt.Errorf("create membership for %q: %w", name, result.Error)
			}
			if result.RowsAffected > 0 {
				audit.LogAction(tx, userID, audit.ActionAddGroupMember, fmt.Sprintf("group:%s", g.ID),
					map[string]any{"origin": "oidc", "user_id": userID})
			}

			desiredGroupIDs = append(desiredGroupIDs, g.ID)
		}

		return nil
	}); err != nil {
		return err
	}

	currentGroupIDs, err := rbacProvider.GetUserGroups(userID)
	if err != nil {
		return fmt.Errorf("casbin list memberships: %w", err)
	}
	currentGroupSet := make(map[uuid.UUID]struct{}, len(currentGroupIDs))
	for _, groupID := range currentGroupIDs {
		currentGroupSet[groupID] = struct{}{}
	}

	addedGroupIDs := make([]uuid.UUID, 0, len(desiredGroupIDs))
	for _, groupID := range desiredGroupIDs {
		if _, ok := currentGroupSet[groupID]; ok {
			continue
		}
		if err := rbacProvider.AddUserToGroup(userID, groupID); err != nil {
			for _, addedGroupID := range addedGroupIDs {
				if removeErr := rbacProvider.RemoveUserFromGroup(userID, addedGroupID); removeErr != nil {
					slog.Error("Failed to roll back Casbin group membership after OIDC sync failure",
						"user_id", userID, "group_id", addedGroupID, "error", removeErr)
				}
			}
			return fmt.Errorf("casbin add %s: %w", groupID, err)
		}
		addedGroupIDs = append(addedGroupIDs, groupID)
	}

	slog.Debug("OIDC groups synced", "user_id", userID, "claim_count", len(desiredNames))
	return nil
}

// findOrCreateOIDCGroup returns the group named name, creating it if needed.
// Creation tolerates a concurrent insert of the same name.
func findOrCreateOIDCGroup(tx *gorm.DB, name string, actorID uuid.UUID) (*models.Group, error) {
	var g models.Group
	err := tx.Where("name = ?", name).First(&g).Error
	if err == nil {
		return &g, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup group %q: %w", name, err)
	}

	g = models.Group{Name: name}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&g)
	if result.Error != nil {
		return nil, fmt.Errorf("create oidc group %q: %w", name, result.Error)
	}
	if result.RowsAffected == 0 {
		var existing models.Group
		if err := tx.Where("name = ?", name).First(&existing).Error; err != nil {
			return nil, fmt.Errorf("lookup group %q: %w", name, err)
		}
		return &existing, nil
	}
	audit.LogAction(tx, actorID, audit.ActionCreateGroup, fmt.Sprintf("group:%s", g.ID),
		map[string]any{"origin": "oidc", "name": g.Name})
	return &g, nil
}

func validateOIDCGroupSyncInputs(db *gorm.DB, rbacProvider rbac.Provider) error {
	if db == nil {
		return errors.New("database is not configured")
	}
	if rbacProvider == nil {
		return errors.New("rbac provider is not configured")
	}

	return nil
}

func normalizedOIDCGroupSet(claimGroups []string) ([]string, map[string]struct{}) {
	desiredNames := normalizeOIDCGroupNames(claimGroups)
	desired := make(map[string]struct{}, len(desiredNames))
	for _, name := range desiredNames {
		desired[name] = struct{}{}
	}

	return desiredNames, desired
}

func syncOIDCGroupRemovalsWithDesired(db *gorm.DB, userID uuid.UUID, desired map[string]struct{}, rbacProvider rbac.Provider) error {
	var current []models.GroupMember
	err := db.
		Where("user_id = ?", userID).
		Preload("Group").
		Find(&current).Error
	if err != nil {
		return fmt.Errorf("list current group memberships: %w", err)
	}

	staleMemberships := staleOIDCMemberships(current, desired)
	return removeOIDCGroupMemberships(db, userID, staleMemberships, rbacProvider)
}

func removeOIDCGroupMemberships(db *gorm.DB, userID uuid.UUID, staleMemberships []models.GroupMember, rbacProvider rbac.Provider) error {
	for _, m := range staleMemberships {
		if err := rbacProvider.RemoveUserFromGroup(userID, m.GroupID); err != nil {
			return fmt.Errorf("casbin remove stale: %w", err)
		}
	}

	if len(staleMemberships) == 0 {
		return nil
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, m := range staleMemberships {
			if err := tx.Where("group_id = ? AND user_id = ?", m.GroupID, userID).Delete(&models.GroupMember{}).Error; err != nil {
				return fmt.Errorf("delete stale membership: %w", err)
			}
			audit.LogAction(tx, userID, audit.ActionRemoveGroupMember, fmt.Sprintf("group:%s", m.GroupID),
				map[string]any{"origin": "oidc", "user_id": userID})
		}

		return nil
	}); err != nil {
		return err
	}
	return nil
}

func staleOIDCMemberships(current []models.GroupMember, desired map[string]struct{}) []models.GroupMember {
	stale := make([]models.GroupMember, 0)
	for _, m := range current {
		if _, ok := desired[m.Group.Name]; ok {
			continue
		}
		stale = append(stale, m)
	}
	return stale
}

func normalizeOIDCGroupNames(claimGroups []string) []string {
	desired := make(map[string]struct{}, len(claimGroups))
	for _, name := range claimGroups {
		// Keycloak's group-membership mapper emits a leading-slash full path
		// ("/developer") when full.path=true and the bare name ("developer")
		// when false. A client can carry both mappers, so the same group
		// arrives twice. Normalize to the bare name so the two forms collapse
		// to one group instead of creating "/developer" and "developer".
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			continue
		}
		desired[name] = struct{}{}
	}

	names := make([]string, 0, len(desired))
	for name := range desired {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
