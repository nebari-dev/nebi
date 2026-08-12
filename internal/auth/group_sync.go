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
)

// syncOIDCGroups reconciles the user's OIDC group memberships with the names
// in the latest ID token's `groups` claim. Idempotent: safe to call on every
// login. Only affects groups with source=oidc; native memberships are
// untouched. Zero-member OIDC groups are preserved so existing workspace
// shares survive churn.
//
// Name collision with native groups: If an OIDC claim names a group that
// already exists with source=native, the membership is NOT added — native
// groups are administered explicitly in nebi, and silently merging IdP claims
// into them would create permanent untracked grants (phase-2 reconcile only
// considers source=oidc memberships).
//
// It also records auth_reconciliation_statuses rows so callers can fail closed
// and operators can see unresolved reconciliation failures. The RBAC provider
// is injected so auth flows can reuse the configured provider and tests can
// fail known reconciliation steps.
func syncOIDCGroups(db *gorm.DB, userID uuid.UUID, claimGroups []string, rbacProvider rbac.Provider) error {
	if err := syncOIDCGroupsOnce(db, userID, claimGroups, rbacProvider); err != nil {
		recordAuthReconciliationFailureWithGroups(db, userID, authReconciliationOIDCGroups, err, claimGroups)
		return err
	}
	if err := recordAuthReconciliationSuccessWithGroups(db, userID, authReconciliationOIDCGroups, claimGroups); err != nil {
		recordAuthReconciliationFailureWithGroups(db, userID, authReconciliationOIDCGroups, err, claimGroups)
		return fmt.Errorf("record oidc group sync success: %w", err)
	}

	return nil
}

func syncOIDCGroupsOnce(db *gorm.DB, userID uuid.UUID, claimGroups []string, rbacProvider rbac.Provider) error {
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
			var g models.Group
			err := tx.Where("name = ?", name).First(&g).Error
			switch {
			case err == nil:
				// If this name already exists as a native group, do NOT merge OIDC claims
				// into it. Native group membership is administered explicitly in nebi; an
				// OIDC claim that happens to share the name must not silently grant
				// permanent access (phase-2 reconcile only looks at source=oidc, so any
				// membership added here would never be removed).
				if g.Source == models.GroupSourceNative {
					slog.Warn("OIDC claim names a native group; skipping membership",
						"group_name", name, "group_id", g.ID, "user_id", userID)
					continue
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				g = models.Group{Name: name, Source: models.GroupSourceOIDC}
				if err := tx.Create(&g).Error; err != nil {
					return fmt.Errorf("create oidc group %q: %w", name, err)
				}
				audit.LogAction(tx, userID, audit.ActionCreateGroup, fmt.Sprintf("group:%s", g.ID),
					map[string]any{"origin": "oidc", "name": g.Name})
			default:
				return fmt.Errorf("lookup group %q: %w", name, err)
			}

			var existing models.GroupMember
			err = tx.Where("group_id = ? AND user_id = ?", g.ID, userID).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&models.GroupMember{GroupID: g.ID, UserID: userID}).Error; err != nil {
					return fmt.Errorf("create membership for %q: %w", name, err)
				}
				audit.LogAction(tx, userID, audit.ActionAddGroupMember, fmt.Sprintf("group:%s", g.ID),
					map[string]any{"origin": "oidc", "user_id": userID})
			} else if err != nil {
				return fmt.Errorf("lookup membership for %q: %w", name, err)
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

func syncOIDCGroupRemovalsOnly(db *gorm.DB, userID uuid.UUID, claimGroups []string, rbacProvider rbac.Provider) error {
	if err := validateOIDCGroupSyncInputs(db, rbacProvider); err != nil {
		return err
	}

	desiredNames, desired := normalizedOIDCGroupSet(claimGroups)
	if err := syncOIDCGroupRemovalsWithDesired(db, userID, desired, rbacProvider); err != nil {
		return err
	}

	slog.Debug("OIDC group removals synced", "user_id", userID, "claim_count", len(desiredNames))
	return nil
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
		Joins("JOIN groups g ON g.id = group_members.group_id").
		Where("group_members.user_id = ? AND g.source = ?", userID, models.GroupSourceOIDC).
		Preload("Group").
		Find(&current).Error
	if err != nil {
		return fmt.Errorf("list current oidc memberships: %w", err)
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
