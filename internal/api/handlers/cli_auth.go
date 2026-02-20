package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/auth"
)

// CLICreateSession registers a pending CLI login session.
// POST /api/v1/auth/cli/session
func CLICreateSession(store *auth.CLISessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Code string `json:"code" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}
		store.Create(req.Code)
		c.JSON(http.StatusCreated, gin.H{"status": "pending"})
	}
}

// CLILogin handles the browser-side of CLI login.
// The user opens this URL in a browser after proxy authentication.
// GET /auth/cli/login?code=...
func CLILogin(basicAuth *auth.BasicAuthenticator, proxyAdminGroups string, store *auth.CLISessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(cliLoginErrorHTML("Missing code parameter.")))
			return
		}

		// Check that the session code exists and is still pending
		sess := store.Get(code)
		if sess == nil {
			c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(cliLoginErrorHTML("Session not found or expired. Please run <code>nebi login</code> again.")))
			return
		}

		// Exchange the proxy IdToken cookie for a Nebi JWT
		resp, err := basicAuth.SessionFromProxy(c.Request, proxyAdminGroups)
		if err != nil {
			c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte(cliLoginErrorHTML("Authentication failed. Make sure you are logged in through the proxy.")))
			return
		}

		username := ""
		if resp.User != nil {
			username = resp.User.Username
		}

		store.Complete(code, resp.Token, username)

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(cliLoginSuccessHTML(username)))
	}
}

// CLIPollToken is polled by the CLI to check if browser auth completed.
// GET /api/v1/auth/cli/token?code=...
func CLIPollToken(store *auth.CLISessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}

		sess := store.Get(code)
		if sess == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found or expired"})
			return
		}

		if sess.Token == "" {
			c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
			return
		}

		// Session complete — return token and clean up
		store.Delete(code)
		c.JSON(http.StatusOK, gin.H{
			"token":    sess.Token,
			"username": sess.Username,
		})
	}
}

func cliLoginSuccessHTML(username string) string {
	greeting := ""
	if username != "" {
		greeting = "<p>Logged in as <strong>" + username + "</strong>.</p>"
	}
	return `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Nebi CLI Login</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         display: flex; justify-content: center; align-items: center; min-height: 100vh;
         margin: 0; background: #f8fafc; color: #1e293b; }
  .card { text-align: center; padding: 3rem; background: white; border-radius: 12px;
          box-shadow: 0 1px 3px rgba(0,0,0,0.1); max-width: 420px; }
  .check { font-size: 3rem; margin-bottom: 1rem; }
  h1 { font-size: 1.25rem; margin: 0 0 0.5rem; }
  p { color: #64748b; margin: 0.25rem 0; }
</style></head>
<body><div class="card">
  <div class="check">&#10003;</div>
  <h1>Authentication complete</h1>
  ` + greeting + `
  <p>You may close this tab and return to the terminal.</p>
</div></body></html>`
}

func cliLoginErrorHTML(msg string) string {
	return `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Nebi CLI Login</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         display: flex; justify-content: center; align-items: center; min-height: 100vh;
         margin: 0; background: #f8fafc; color: #1e293b; }
  .card { text-align: center; padding: 3rem; background: white; border-radius: 12px;
          box-shadow: 0 1px 3px rgba(0,0,0,0.1); max-width: 420px; }
  .icon { font-size: 3rem; margin-bottom: 1rem; }
  h1 { font-size: 1.25rem; margin: 0 0 0.5rem; }
  p { color: #ef4444; margin: 0.25rem 0; }
</style></head>
<body><div class="card">
  <div class="icon">&#10007;</div>
  <h1>Login failed</h1>
  <p>` + msg + `</p>
</div></body></html>`
}
