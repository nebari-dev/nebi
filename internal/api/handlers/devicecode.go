package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/auth"
)

// Device code store — in-memory, no external dependencies.

const (
	deviceCodeTTL     = 5 * time.Minute
	deviceCodeCleanup = 60 * time.Second
	deviceCodeLength  = 8 // e.g., "ABCD-1234"
)

type deviceCodeEntry struct {
	code      string
	token     string
	username  string
	completed bool
	expiresAt time.Time
}

type deviceCodeStore struct {
	mu      sync.Mutex
	entries map[string]*deviceCodeEntry
}

var defaultCodeStore = newDeviceCodeStore()

func newDeviceCodeStore() *deviceCodeStore {
	s := &deviceCodeStore{
		entries: make(map[string]*deviceCodeEntry),
	}
	go s.cleanupLoop()
	return s
}

func (s *deviceCodeStore) cleanupLoop() {
	ticker := time.NewTicker(deviceCodeCleanup)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for code, entry := range s.entries {
			if now.After(entry.expiresAt) {
				delete(s.entries, code)
			}
		}
		s.mu.Unlock()
	}
}

// Generate creates a new device code and stores it.
func (s *deviceCodeStore) Generate() (string, error) {
	code, err := generateCode()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[code] = &deviceCodeEntry{
		code:      code,
		expiresAt: time.Now().Add(deviceCodeTTL),
	}
	return code, nil
}

// Complete marks a device code as completed with the auth result.
func (s *deviceCodeStore) Complete(code, token, username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[code]
	if !ok || time.Now().After(entry.expiresAt) {
		return false
	}
	entry.token = token
	entry.username = username
	entry.completed = true
	return true
}

// Poll checks the status of a device code.
func (s *deviceCodeStore) Poll(code string) (token, username string, found, completed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[code]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", "", false, false
	}
	return entry.token, entry.username, true, entry.completed
}

// generateCode creates a code like "ABCD-1234" (4 uppercase letters + 4 digits).
func generateCode() (string, error) {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // no I, O (avoid confusion with 1, 0)
	const digits = "0123456789"

	code := make([]byte, 9) // 4 letters + dash + 4 digits
	for i := 0; i < 4; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		code[i] = letters[n.Int64()]
	}
	code[4] = '-'
	for i := 5; i < 9; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		code[i] = digits[n.Int64()]
	}
	return string(code), nil
}

// --- Handlers ---

// CLILoginCode godoc
// @Summary Request a device code for CLI login
// @Description Generates a short-lived device code for browser-based CLI authentication.
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /auth/cli-login/code [post]
func CLILoginCode() gin.HandlerFunc {
	return func(c *gin.Context) {
		code, err := defaultCodeStore.Generate()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate code"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":       code,
			"expires_in": int(deviceCodeTTL.Seconds()),
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
func CLILogin(basicAuth *auth.BasicAuthenticator, proxyAdminGroups string) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}

		// Verify the code exists and hasn't expired
		_, _, found, completed := defaultCodeStore.Poll(code)
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "unknown or expired code"})
			return
		}
		if completed {
			renderCLISuccess(c)
			return
		}

		// Try proxy session first (OIDC proxy sets an IdToken cookie)
		resp, err := basicAuth.SessionFromProxy(c.Request, proxyAdminGroups)
		if err == nil {
			defaultCodeStore.Complete(code, resp.Token, resp.User.Username)
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

			defaultCodeStore.Complete(code, loginResp.Token, loginResp.User.Username)
			renderCLISuccess(c)
			return
		}

		// GET with no proxy session — show login form
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
func CLILoginPoll() gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}

		token, username, found, completed := defaultCodeStore.Poll(code)
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
