package handlers

import (
	"errors"
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

// SessionRedirect exchanges a proxy IdToken cookie for a Nebi JWT and
// redirects to the login page with the token as a query parameter. This
// endpoint lives outside /api/ so it goes through the gateway's protected
// route (which preserves OIDC cookies set by Envoy Gateway). The /api/
// routes are public and Envoy strips OIDC cookies from them.
func SessionRedirect(basicAuth *auth.BasicAuthenticator, proxyAdminGroups string, basePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := basicAuth.SessionFromProxy(c.Request, proxyAdminGroups)
		if err != nil {
			// No valid proxy session — redirect to login without token
			c.Redirect(http.StatusFound, basePath+"/login")
			return
		}
		c.Redirect(http.StatusFound, basePath+"/login?token="+resp.Token)
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
