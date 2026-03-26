package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/auth"
)

// DeviceConfigResponse is the JSON response from GET /auth/device-config.
type DeviceConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	IssuerURL string `json:"issuer_url,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
}

// DeviceConfig returns the OIDC device flow configuration for CLI clients.
// If device flow is not configured, returns {"enabled": false}.
func DeviceConfig(issuerURL, deviceClientID string) gin.HandlerFunc {
	resp := DeviceConfigResponse{Enabled: false}
	if issuerURL != "" && deviceClientID != "" {
		resp = DeviceConfigResponse{
			Enabled:   true,
			IssuerURL: issuerURL,
			ClientID:  deviceClientID,
		}
	}
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, resp)
	}
}

// DeviceTokenRequest is the JSON body for POST /auth/device-token.
type DeviceTokenRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}

// DeviceToken exchanges a Keycloak ID token (from device flow) for a Nebi JWT.
// The ID token is verified using the OIDC provider's JWKS, then the user is
// found/created and a Nebi JWT is returned.
func DeviceToken(basicAuth *auth.BasicAuthenticator, adminGroups string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DeviceTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id_token is required"})
			return
		}

		resp, err := basicAuth.ExchangeIDToken(req.IDToken, adminGroups)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired id_token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":    resp.Token,
			"username": resp.User.Username,
		})
	}
}
