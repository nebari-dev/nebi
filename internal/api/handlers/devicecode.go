package handlers

import (
	"fmt"
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
func CLILogin(basicAuth *auth.BasicAuthenticator, proxyAdminGroups string, store *auth.DeviceCodeStore) gin.HandlerFunc {
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

		// Try gateway token first (Envoy forwards access token via Authorization header)
		resp, err := basicAuth.SessionFromGateway(c.Request, proxyAdminGroups)
		if err == nil {
			store.Complete(code, resp.Token, resp.User.Username)
			renderCLISuccess(c)
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

		// GET with no gateway token — show login form
		renderCLILoginForm(c, code, "")
	}
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
