package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/auth"
)

// isRequestHTTPS reports whether the request reached Nebi over HTTPS, as seen
// by the client. It trusts the terminating proxy's X-Forwarded-Proto header
// (the common Kubernetes/ingress deployment terminates TLS upstream) and falls
// back to the direct TLS connection state. Used to set the Secure flag on
// auth cookies from the externally visible origin, not just backend transport.
func isRequestHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

// setStateCookie writes the OIDC state cookie with hardened flags: HttpOnly,
// SameSite=Lax (the IdP callback is a top-level GET redirect, which Lax still
// carries; Strict would drop it), and Secure whenever the request origin is
// HTTPS. Pass maxAge -1 to clear the cookie. Both login and callback go
// through here so the flags can never diverge.
func setStateCookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("oidc_state", value, maxAge, "/", "", isRequestHTTPS(c), true)
}

// OIDCLogin godoc
// @Summary Initiate OIDC login
// @Description Redirects user to OIDC provider for authentication
// @Tags auth
// @Produce json
// @Success 302 {string} string "Redirect to OIDC provider"
// @Router /auth/oidc/login [get]
func OIDCLogin(oidcAuth *auth.OIDCAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate random state for CSRF protection
		state, err := generateRandomState()
		if err != nil {
			slog.Error("Failed to generate state", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		// Store state in session/cookie (for simplicity, we'll use a cookie)
		setStateCookie(c, state, 600)

		// Get auth URL and redirect
		authURL := oidcAuth.GetAuthURL(state)
		c.Redirect(http.StatusTemporaryRedirect, authURL)
	}
}

// OIDCCallback godoc
// @Summary Handle OIDC callback
// @Description Process OIDC callback and authenticate user
// @Tags auth
// @Accept json
// @Produce json
// @Param code query string true "Authorization code"
// @Param state query string true "State parameter"
// @Success 200 {object} auth.LoginResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /auth/oidc/callback [get]
func OIDCCallback(oidcAuth *auth.OIDCAuthenticator, codeStore *auth.AuthCodeStore, basePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Verify state to prevent CSRF
		state := c.Query("state")
		storedState, err := c.Cookie("oidc_state")
		if err != nil || state == "" || state != storedState {
			slog.Warn("Invalid OIDC state", "had_state_cookie", err == nil)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state parameter"})
			return
		}

		// Clear the state cookie
		setStateCookie(c, "", -1)

		// Get authorization code from OIDC provider
		oidcCode := c.Query("code")
		if oidcCode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
			return
		}

		// Exchange OIDC authorization code for tokens
		resp, err := oidcAuth.HandleCallback(c.Request.Context(), oidcCode)
		if err != nil {
			slog.Error("OIDC callback failed", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, basePath+"/login?error=oauth_failed")
			return
		}

		// Generate a single-use Nebi authorization code instead of putting
		// the JWT in the URL (RFC 6749 §4.1 pattern).
		userJSON, err := json.Marshal(resp.User)
		if err != nil {
			slog.Error("Failed to marshal user", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, basePath+"/login?error=oauth_failed")
			return
		}

		authCode, err := codeStore.Generate(resp.Token, userJSON)
		if err != nil {
			slog.Error("Failed to generate auth code", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, basePath+"/login?error=oauth_failed")
			return
		}

		c.Redirect(http.StatusTemporaryRedirect, basePath+"/login?code="+authCode)
	}
}

// generateRandomState generates a random state string for CSRF protection
func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
