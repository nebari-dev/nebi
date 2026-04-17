package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// ProxyTokenClaims represents claims extracted from an IdToken cookie
// set by an authenticating proxy (e.g., Envoy Gateway after Keycloak OIDC).
type ProxyTokenClaims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Picture           string   `json:"picture"`
	Groups            []string `json:"groups"`
}

// verifyIdTokenCookie finds a cookie whose name starts with "IdToken" and
// verifies its signature using the OIDC provider's JWKS before extracting claims.
func verifyIdTokenCookie(r *http.Request, verifier *oidc.IDTokenVerifier) (*ProxyTokenClaims, error) {
	if verifier == nil {
		return nil, errors.New("IdToken verification not configured")
	}

	var rawToken string
	for _, c := range r.Cookies() {
		if strings.HasPrefix(c.Name, "IdToken") {
			rawToken = c.Value
			break
		}
	}
	if rawToken == "" {
		return nil, errors.New("no IdToken cookie found")
	}

	idToken, err := verifier.Verify(r.Context(), rawToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify IdToken: %w", err)
	}

	var claims ProxyTokenClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to extract claims from verified IdToken: %w", err)
	}

	return &claims, nil
}

// findOrCreateProxyUser looks up a user by username or email from proxy
// claims. If no user exists, one is created. Avatar is updated on every call.
func findOrCreateProxyUser(db *gorm.DB, claims *ProxyTokenClaims) (*models.User, error) {
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = claims.Sub
	}
	if username == "" {
		return nil, errors.New("proxy token has no usable identity claim")
	}

	email := claims.Email
	if email == "" {
		email = username + "@proxy.local"
	}

	var user models.User
	result := db.Where("username = ? OR email = ?", username, email).First(&user)
	if result.Error == nil {
		// Existing user — update avatar if changed
		if user.AvatarURL != claims.Picture {
			user.AvatarURL = claims.Picture
			db.Save(&user)
		}
		return &user, nil
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error: %w", result.Error)
	}

	// Create new user
	user = models.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        email,
		AvatarURL:    claims.Picture,
		PasswordHash: "", // proxy users don't have passwords
	}
	if err := db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to create proxy user: %w", err)
	}

	slog.Info("Created new user from proxy auth", "user_id", user.ID, "username", user.Username, "email", email)
	return &user, nil
}

// syncRolesFromGroups grants or revokes Nebi admin based on whether the
// user belongs to any of the configured admin groups.
func syncRolesFromGroups(userID uuid.UUID, groups []string, adminGroups []string, rbacProvider rbac.Provider) {
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

	isAdmin, err := rbacProvider.IsAdmin(userID)
	if err != nil {
		slog.Warn("Failed to check admin status during proxy sync", "user_id", userID, "error", err)
		return
	}

	if shouldBeAdmin && !isAdmin {
		if err := rbacProvider.MakeAdmin(userID); err != nil {
			slog.Warn("Failed to grant admin from proxy groups", "user_id", userID, "error", err)
		} else {
			slog.Info("Granted admin via proxy group membership", "user_id", userID)
		}
	} else if !shouldBeAdmin && isAdmin {
		if err := rbacProvider.RevokeAdmin(userID); err != nil {
			slog.Warn("Failed to revoke admin from proxy groups", "user_id", userID, "error", err)
		} else {
			slog.Info("Revoked admin via proxy group membership", "user_id", userID)
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
