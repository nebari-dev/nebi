package auth

import (
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/rbac"
)

// syncRolesFromGroups grants or revokes Nebi admin based on whether the
// user belongs to any of the configured admin groups.
func syncRolesFromGroups(userID uuid.UUID, groups []string, adminGroups []string) {
	adminGroupSet := make(map[string]bool, len(adminGroups))
	for _, g := range adminGroups {
		g = strings.TrimSpace(g)
		if g != "" {
			adminGroupSet[g] = true
		}
	}

	shouldBeAdmin := false
	for _, g := range groups {
		// Strip leading "/" that Keycloak sometimes adds
		g = strings.TrimPrefix(g, "/")
		if adminGroupSet[g] {
			shouldBeAdmin = true
			break
		}
	}

	isAdmin, err := rbac.IsAdmin(userID)
	if err != nil {
		slog.Warn("Failed to check admin status during role sync", "user_id", userID, "error", err)
		return
	}

	if shouldBeAdmin && !isAdmin {
		if err := rbac.MakeAdmin(userID); err != nil {
			slog.Warn("Failed to grant admin from OIDC groups", "user_id", userID, "error", err)
		} else {
			slog.Info("Granted admin via OIDC group membership", "user_id", userID)
		}
	} else if !shouldBeAdmin && isAdmin {
		if err := rbac.RevokeAdmin(userID); err != nil {
			slog.Warn("Failed to revoke admin from OIDC groups", "user_id", userID, "error", err)
		} else {
			slog.Info("Revoked admin via OIDC group membership", "user_id", userID)
		}
	}
}

// parseAdminGroups splits a comma-separated string into a slice of group names.
func parseAdminGroups(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
