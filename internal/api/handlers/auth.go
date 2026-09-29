package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AuthConfigResponse tells clients how to authenticate. Nebi does not issue
// tokens: with type "oidc", clients obtain an access token from the issuer
// (authorization code + PKCE in the browser, device authorization grant in
// the CLI and desktop app) for client_id and send it as a bearer token. With
// type "none", no credentials are needed.
type AuthConfigResponse struct {
	Type      string   `json:"type" example:"oidc"`
	IssuerURL string   `json:"issuer_url,omitempty" example:"https://auth.example.com/realms/nebi"`
	ClientID  string   `json:"client_id,omitempty" example:"nebi"`
	Scopes    []string `json:"scopes,omitempty"`
}

// AuthConfig godoc
// @Summary Get authentication configuration
// @Description Returns the OIDC provider and client that issue access tokens for this server, or type "none" when authentication is disabled
// @Tags auth
// @Produce json
// @Success 200 {object} AuthConfigResponse
// @Router /auth/config [get]
func AuthConfig(resp AuthConfigResponse) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, resp)
	}
}
