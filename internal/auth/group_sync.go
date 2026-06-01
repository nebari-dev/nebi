package auth

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// SyncOIDCGroups reconciles the user's OIDC group memberships with the names
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
func SyncOIDCGroups(db *gorm.DB, userID uuid.UUID, claimGroups []string) error {
	desired := make(map[string]struct{}, len(claimGroups))
	for _, name := range claimGroups {
		if name == "" {
			continue
		}
		desired[name] = struct{}{}
	}

	// Phase 1: upsert each desired group + membership.
	for name := range desired {
		var g models.Group
		err := db.Where("name = ?", name).First(&g).Error
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
			if err := db.Create(&g).Error; err != nil {
				return fmt.Errorf("create oidc group %q: %w", name, err)
			}
			audit.LogAction(db, userID, audit.ActionCreateGroup, fmt.Sprintf("group:%s", g.ID),
				map[string]any{"origin": "oidc", "name": g.Name})
		default:
			return fmt.Errorf("lookup group %q: %w", name, err)
		}

		var existing models.GroupMember
		err = db.Where("group_id = ? AND user_id = ?", g.ID, userID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&models.GroupMember{GroupID: g.ID, UserID: userID}).Error; err != nil {
				return fmt.Errorf("create membership for %q: %w", name, err)
			}
			audit.LogAction(db, userID, audit.ActionAddGroupMember, fmt.Sprintf("group:%s", g.ID),
				map[string]any{"origin": "oidc", "user_id": userID})
		} else if err != nil {
			return fmt.Errorf("lookup membership for %q: %w", name, err)
		}

		if err := rbac.AddUserToGroup(userID, g.ID); err != nil {
			return fmt.Errorf("casbin add %q: %w", name, err)
		}
	}

	// Phase 2: remove stale OIDC memberships not in claim.
	var current []models.GroupMember
	err := db.
		Joins("JOIN groups g ON g.id = group_members.group_id").
		Where("group_members.user_id = ? AND g.source = ?", userID, models.GroupSourceOIDC).
		Preload("Group").
		Find(&current).Error
	if err != nil {
		return fmt.Errorf("list current oidc memberships: %w", err)
	}

	for _, m := range current {
		if _, ok := desired[m.Group.Name]; ok {
			continue
		}
		if err := db.Where("group_id = ? AND user_id = ?", m.GroupID, userID).Delete(&models.GroupMember{}).Error; err != nil {
			return fmt.Errorf("delete stale membership: %w", err)
		}
		audit.LogAction(db, userID, audit.ActionRemoveGroupMember, fmt.Sprintf("group:%s", m.GroupID),
			map[string]any{"origin": "oidc", "user_id": userID})
		if err := rbac.RemoveUserFromGroup(userID, m.GroupID); err != nil {
			return fmt.Errorf("casbin remove stale: %w", err)
		}
	}

	slog.Debug("OIDC groups synced", "user_id", userID, "claim_count", len(desired))
	return nil
}
