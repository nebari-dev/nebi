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
	Issuer            string   `json:"-"`
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
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
	claims.Issuer = idToken.Issuer
	if claims.Sub == "" {
		claims.Sub = idToken.Subject
	}

	return &claims, nil
}

// findOrCreateProxyUser finds or creates the local user bound to the verified
// proxy token issuer and subject.
func findOrCreateProxyUser(db *gorm.DB, claims *ProxyTokenClaims) (*models.User, error) {
	if claims == nil {
		return nil, errors.New("proxy token has no claims")
	}
	return findOrCreateFederatedUser(db, federatedUserClaims{
		Issuer:            claims.Issuer,
		Subject:           claims.Sub,
		PreferredUsername: claims.PreferredUsername,
		Email:             claims.Email,
		EmailVerified:     claims.EmailVerified,
		Name:              claims.Name,
		AvatarURL:         claims.Picture,
	})
}

func shouldBeAdminFromGroups(groups []string, adminGroups []string) bool {
	adminGroupSet := make(map[string]bool, len(adminGroups))
	for _, g := range adminGroups {
		g = strings.TrimSpace(g)
		if g != "" {
			adminGroupSet[g] = true
		}
	}

	for _, g := range groups {
		// Strip leading "/" that Keycloak sometimes adds
		g = strings.TrimPrefix(g, "/")
		if adminGroupSet[g] {
			return true
		}
	}

	return false
}

func syncAdminRoleToDesired(userID uuid.UUID, shouldBeAdmin bool, rbacProvider rbac.Provider) error {
	if rbacProvider == nil {
		return errors.New("rbac provider is not configured")
	}

	isAdmin, err := rbacProvider.IsAdmin(userID)
	if err != nil {
		return fmt.Errorf("check admin status during proxy sync: %w", err)
	}

	if shouldBeAdmin && !isAdmin {
		if err := rbacProvider.MakeAdmin(userID); err != nil {
			return fmt.Errorf("grant admin from proxy groups: %w", err)
		} else {
			slog.Info("Granted admin via proxy group membership", "user_id", userID)
		}
	} else if !shouldBeAdmin && isAdmin {
		if err := rbacProvider.RevokeAdmin(userID); err != nil {
			return fmt.Errorf("revoke admin from proxy groups: %w", err)
		} else {
			slog.Info("Revoked admin via proxy group membership", "user_id", userID)
		}
	}

	return nil
}

func (a *BasicAuthenticator) syncProxyAdminRole(userID uuid.UUID, groups []string, adminGroups []string) error {
	shouldBeAdmin := shouldBeAdminFromGroups(groups, adminGroups)
	if err := syncAdminRoleToDesired(userID, shouldBeAdmin, a.rbac); err != nil {
		recordAuthReconciliationFailureWithAdmin(a.db, userID, authReconciliationProxyAdmin, err, shouldBeAdmin)
		return err
	}
	if err := recordAuthReconciliationSuccessWithAdmin(a.db, userID, authReconciliationProxyAdmin, shouldBeAdmin); err != nil {
		recordAuthReconciliationFailureWithAdmin(a.db, userID, authReconciliationProxyAdmin, err, shouldBeAdmin)
		return fmt.Errorf("record proxy admin sync success: %w", err)
	}
	return nil
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
