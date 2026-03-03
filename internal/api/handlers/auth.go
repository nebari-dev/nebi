package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/auth"
)

// Login godoc
// @Summary User login
// @Description Authenticate user and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body auth.LoginRequest true "Login credentials"
// @Success 200 {object} auth.LoginResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /auth/login [post]
func Login(authenticator auth.Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req auth.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		resp, err := authenticator.Login(req.Username, req.Password)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// SessionCheck godoc
// @Summary Check proxy session
// @Description Check for an IdToken cookie (set by an authenticating proxy) and return a Nebi JWT
// @Tags auth
// @Produce json
// @Success 200 {object} auth.LoginResponse
// @Failure 401 {object} map[string]string
// @Router /auth/session [get]
func SessionCheck(basicAuth *auth.BasicAuthenticator, proxyAdminGroups string) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := basicAuth.SessionFromProxy(c.Request, proxyAdminGroups)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no proxy session"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// CLILogin godoc
// @Summary Browser-based login for CLI clients behind an OIDC proxy
// @Description After the user authenticates via the OIDC proxy (e.g., Envoy Gateway + Keycloak),
// @Description this endpoint reads the IdToken cookie, exchanges it for a Nebi JWT, and renders
// @Description a page that sends the token to the CLI's local callback server.
// @Tags auth
// @Produce html
// @Param callback_port query int true "CLI's local callback server port"
// @Success 200 {string} string "HTML page that redirects token to CLI"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /auth/cli-login [get]
func CLILogin(basicAuth *auth.BasicAuthenticator, proxyAdminGroups string) gin.HandlerFunc {
	return func(c *gin.Context) {
		callbackPort := c.Query("callback_port")
		if callbackPort == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "callback_port is required"})
			return
		}

		// Exchange the IdToken cookie (set by Envoy/Keycloak) for a Nebi JWT
		resp, err := basicAuth.SessionFromProxy(c.Request, proxyAdminGroups)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no proxy session — are you behind an OIDC proxy?"})
			return
		}

		// Render a page that sends the token to the CLI's local callback server
		callbackURL := fmt.Sprintf("http://localhost:%s/callback?token=%s&username=%s",
			callbackPort, resp.Token, resp.User.Username)

		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <title>Nebi CLI Login</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f8f9fa; }
    .card { background: white; border-radius: 12px; padding: 2rem; box-shadow: 0 2px 8px rgba(0,0,0,0.1); text-align: center; max-width: 400px; }
    .success { color: #059669; font-size: 1.2rem; }
    .spinner { display: inline-block; width: 20px; height: 20px; border: 3px solid #e5e7eb; border-top-color: #3b82f6; border-radius: 50%%; animation: spin 0.8s linear infinite; margin-right: 8px; }
    @keyframes spin { to { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <div class="card">
    <div id="status"><span class="spinner"></span>Completing login...</div>
  </div>
  <script>
    fetch("%s", { mode: "no-cors" })
      .then(function() {
        document.getElementById("status").innerHTML = '<p class="success">Login successful! You can close this tab.</p>';
      })
      .catch(function() {
        document.getElementById("status").innerHTML = '<p class="success">Login successful! You can close this tab.</p>';
      });
  </script>
</body>
</html>`, callbackURL))
	}
}
