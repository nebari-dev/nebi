package handlers

import (
	"encoding/json"
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

// SessionRedirect exchanges a proxy IdToken cookie for a short-lived,
// single-use authorization code and redirects to /login?code=<code>.
// The frontend then exchanges the code for a JWT via POST /api/v1/auth/code/exchange.
//
// This follows the OAuth 2.0 authorization code pattern (RFC 6749 §4.1):
// sensitive tokens never appear in URLs, logs, or browser history.
//
// This endpoint lives outside /api/ so that gateway proxies that strip
// cookies from public routes still forward them here.
func SessionRedirect(basicAuth *auth.BasicAuthenticator, proxyAdminGroups string, basePath string, codeStore *auth.AuthCodeStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := basicAuth.SessionFromProxy(c.Request, proxyAdminGroups)
		if err != nil {
			// No valid proxy session — redirect to login without code
			c.Redirect(http.StatusFound, basePath+"/login")
			return
		}

		userJSON, err := json.Marshal(resp.User)
		if err != nil {
			c.Redirect(http.StatusFound, basePath+"/login")
			return
		}

		code, err := codeStore.Generate(resp.Token, userJSON)
		if err != nil {
			c.Redirect(http.StatusFound, basePath+"/login")
			return
		}

		c.Redirect(http.StatusFound, basePath+"/login?code="+code)
	}
}

// CodeExchange exchanges a single-use authorization code for a Nebi JWT.
// The code is consumed on use and expires after 30 seconds.
func CodeExchange(codeStore *auth.AuthCodeStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Code string `json:"code"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code"})
			return
		}

		token, userJSON, ok := codeStore.Exchange(req.Code)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired code"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"token": token, "user": json.RawMessage(userJSON)})
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
