package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"

	"github.com/openteams-ai/darb/internal/auth"
	"github.com/gin-gonic/gin"
)

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
		c.SetCookie("oidc_state", state, 600, "/", "", false, true)

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
func OIDCCallback(oidcAuth *auth.OIDCAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Verify state to prevent CSRF
		state := c.Query("state")
		storedState, err := c.Cookie("oidc_state")
		if err != nil || state == "" || state != storedState {
			slog.Warn("Invalid OIDC state", "state", state, "stored_state", storedState)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state parameter"})
			return
		}

		// Clear the state cookie
		c.SetCookie("oidc_state", "", -1, "/", "", false, true)

		// Get authorization code
		code := c.Query("code")
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
			return
		}

		// Handle callback
		resp, err := oidcAuth.HandleCallback(c.Request.Context(), code)
		if err != nil {
			slog.Error("OIDC callback failed", "error", err)
			// Redirect to login with error
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oauth_failed")
			return
		}

		// Redirect to frontend with token as query parameter
		// The frontend will store it and redirect to home
		c.Redirect(http.StatusTemporaryRedirect, "/login?token="+resp.Token)
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
