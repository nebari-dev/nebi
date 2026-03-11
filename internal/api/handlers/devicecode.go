package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/auth"
)

// CLILoginCode godoc
// @Summary Request a device code for CLI login
// @Description Generates a short-lived device code for browser-based CLI authentication.
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /auth/cli-login/code [post]
func CLILoginCode(store *auth.DeviceCodeStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		code, err := store.Generate()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate code"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":       code,
			"expires_in": store.TTLSeconds(),
		})
	}
}

// CLILogin godoc
// @Summary Browser-based login for CLI clients
// @Description Handles browser-based CLI login using a device code flow.
// @Description If behind an OIDC proxy, auto-completes the code. Otherwise shows a login form.
// @Tags auth
// @Produce html
// @Param code query string true "Device code from CLI"
// @Success 200 {string} string "HTML page"
// @Failure 400 {object} map[string]string
// @Router /auth/cli-login [get]
// cliLoginState embeds the device code in the OIDC state parameter.
type cliLoginState struct {
	Nonce      string `json:"n"`
	DeviceCode string `json:"c"`
}

func CLILogin(basicAuth *auth.BasicAuthenticator, store *auth.DeviceCodeStore, oidcAuth *auth.OIDCAuthenticator, cliCallbackURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}

		// Verify the code exists and hasn't expired
		_, _, found, completed := store.Poll(code)
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "unknown or expired code"})
			return
		}
		if completed {
			renderCLISuccess(c)
			return
		}

		// If OIDC is available and this is a GET, redirect to Keycloak
		if oidcAuth != nil && c.Request.Method == http.MethodGet {
			nonce, err := generateRandomState()
			if err != nil {
				slog.Error("Failed to generate state for CLI login", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
				return
			}

			stateData := cliLoginState{Nonce: nonce, DeviceCode: code}
			stateJSON, _ := json.Marshal(stateData)
			state := base64.URLEncoding.EncodeToString(stateJSON)

			c.SetCookie("cli_login_state", state, 600, "/", "", false, true)
			authURL := oidcAuth.GetAuthURLWithRedirect(state, cliCallbackURL)
			c.Redirect(http.StatusTemporaryRedirect, authURL)
			return
		}

		if c.Request.Method == http.MethodPost {
			username := c.PostForm("username")
			password := c.PostForm("password")

			loginResp, loginErr := basicAuth.Login(username, password)
			if loginErr != nil {
				renderCLILoginForm(c, code, "Invalid username or password")
				return
			}

			store.Complete(code, loginResp.Token, loginResp.User.Username)
			renderCLISuccess(c)
			return
		}

		// GET without OIDC — show login form
		renderCLILoginForm(c, code, "")
	}
}

// CLILoginCallback handles the OIDC callback for the device code flow.
func CLILoginCallback(oidcAuth *auth.OIDCAuthenticator, store *auth.DeviceCodeStore, cliCallbackURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Verify state cookie for CSRF
		state := c.Query("state")
		storedState, err := c.Cookie("cli_login_state")
		if err != nil || state == "" || state != storedState {
			slog.Warn("Invalid CLI login state", "state", state)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state parameter"})
			return
		}
		c.SetCookie("cli_login_state", "", -1, "/", "", false, true)

		// Extract device code from state
		stateJSON, err := base64.URLEncoding.DecodeString(state)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state encoding"})
			return
		}
		var stateData cliLoginState
		if err := json.Unmarshal(stateJSON, &stateData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state data"})
			return
		}

		// Verify device code is still valid
		_, _, found, completed := store.Poll(stateData.DeviceCode)
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "device code expired"})
			return
		}
		if completed {
			renderCLISuccess(c)
			return
		}

		// Exchange auth code for tokens
		authCode := c.Query("code")
		if authCode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
			return
		}

		resp, err := oidcAuth.HandleCallbackWithRedirect(c.Request.Context(), authCode, cliCallbackURL)
		if err != nil {
			slog.Error("CLI login OIDC callback failed", "error", err)
			renderCLIError(c, "Authentication failed")
			return
		}

		store.Complete(stateData.DeviceCode, resp.Token, resp.User.Username)
		renderCLISuccess(c)
	}
}

// renderCLIError renders an error page for CLI login.
func renderCLIError(c *gin.Context, msg string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <title>Nebi CLI Login</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f8f9fa; }
    .card { background: white; border-radius: 12px; padding: 2rem; box-shadow: 0 2px 8px rgba(0,0,0,0.1); text-align: center; max-width: 400px; }
    .error { color: #dc2626; font-size: 1.2rem; }
  </style>
</head>
<body>
  <div class="card">
    <p class="error">%s</p>
    <p style="color: #6b7280; font-size: 0.9rem;">Please close this tab and try again.</p>
  </div>
</body>
</html>`, msg))
}

// CLILoginPoll godoc
// @Summary Poll for CLI login completion
// @Description Polls the status of a device code. Returns the token when authentication is complete.
// @Tags auth
// @Produce json
// @Param code query string true "Device code"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Router /auth/cli-login/poll [get]
func CLILoginPoll(store *auth.DeviceCodeStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}

		token, username, found, completed := store.Poll(code)
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "unknown or expired code"})
			return
		}

		if !completed {
			c.JSON(http.StatusOK, gin.H{"status": "pending"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "complete",
			"token":    token,
			"username": username,
		})
	}
}

// renderCLISuccess renders a simple success page after browser authentication.
func renderCLISuccess(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!DOCTYPE html>
<html>
<head>
  <title>Nebi CLI Login</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f8f9fa; }
    .card { background: white; border-radius: 12px; padding: 2rem; box-shadow: 0 2px 8px rgba(0,0,0,0.1); text-align: center; max-width: 400px; }
    .success { color: #059669; font-size: 1.2rem; }
  </style>
</head>
<body>
  <div class="card">
    <p class="success">Login successful! You can close this tab.</p>
    <p style="color: #6b7280; font-size: 0.9rem;">Your CLI session is now authenticated.</p>
  </div>
</body>
</html>`)
}

// renderCLILoginForm renders a login form for CLI browser-based authentication.
func renderCLILoginForm(c *gin.Context, code, errMsg string) {
	errorHTML := ""
	if errMsg != "" {
		errorHTML = fmt.Sprintf(`<div class="error">%s</div>`, errMsg)
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <title>Nebi CLI Login</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f8f9fa; }
    .card { background: white; border-radius: 12px; padding: 2rem; box-shadow: 0 2px 8px rgba(0,0,0,0.1); text-align: center; max-width: 400px; width: 100%%; }
    h2 { margin: 0 0 0.5rem; color: #1f2937; }
    .code { font-family: monospace; font-size: 1.1rem; color: #3b82f6; background: #eff6ff; padding: 0.25rem 0.75rem; border-radius: 4px; margin-bottom: 1.5rem; display: inline-block; }
    .form-group { margin-bottom: 1rem; text-align: left; }
    label { display: block; margin-bottom: 0.25rem; color: #374151; font-size: 0.9rem; font-weight: 500; }
    input[type="text"], input[type="password"] { width: 100%%; padding: 0.5rem 0.75rem; border: 1px solid #d1d5db; border-radius: 6px; font-size: 1rem; box-sizing: border-box; }
    input:focus { outline: none; border-color: #3b82f6; box-shadow: 0 0 0 3px rgba(59,130,246,0.1); }
    button { width: 100%%; padding: 0.6rem; background: #3b82f6; color: white; border: none; border-radius: 6px; font-size: 1rem; cursor: pointer; margin-top: 0.5rem; }
    button:hover { background: #2563eb; }
    .error { color: #dc2626; background: #fef2f2; border: 1px solid #fecaca; padding: 0.5rem 0.75rem; border-radius: 6px; font-size: 0.9rem; margin-bottom: 1rem; }
  </style>
</head>
<body>
  <div class="card">
    <h2>Nebi CLI Login</h2>
    <div class="code">%s</div>
    %s
    <form method="POST">
      <div class="form-group">
        <label for="username">Username</label>
        <input type="text" id="username" name="username" required autocomplete="username" autofocus />
      </div>
      <div class="form-group">
        <label for="password">Password</label>
        <input type="password" id="password" name="password" required autocomplete="current-password" />
      </div>
      <button type="submit">Log In</button>
    </form>
  </div>
</body>
</html>`, code, errorHTML))
}
