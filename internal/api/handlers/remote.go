package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
)

// RemoteHandler proxies requests to a remote Nebi server.
// Used in local/desktop mode so the frontend can browse remote servers.
type RemoteHandler struct {
	db *gorm.DB
}

// NewRemoteHandler creates a new remote handler.
func NewRemoteHandler(db *gorm.DB) *RemoteHandler {
	return &RemoteHandler{db: db}
}

// getClient builds a cliclient.Client from the stored server URL and credentials.
func (h *RemoteHandler) getClient() (*cliclient.Client, error) {
	var cfg store.Config
	if err := h.db.First(&cfg).Error; err != nil {
		return nil, fmt.Errorf("no server configured")
	}
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("no server URL configured")
	}
	var creds store.Credentials
	if err := h.db.First(&creds).Error; err != nil || creds.Token == "" {
		return nil, fmt.Errorf("not authenticated with remote server")
	}
	return cliclient.New(cfg.ServerURL, creds.Token), nil
}

// notConnected returns 503 when no remote server is configured.
func (h *RemoteHandler) notConnected(c *gin.Context, err error) {
	c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
}

// ConnectServer authenticates with a remote server and stores credentials.
func (h *RemoteHandler) ConnectServer(c *gin.Context) {
	var req struct {
		URL      string `json:"url" binding:"required"`
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Login to remote server
	client := cliclient.NewWithoutAuth(req.URL)
	loginResp, err := client.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("Failed to connect: %v", err)})
		return
	}

	// Store URL and credentials
	if err := h.db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", req.URL).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to store server URL"})
		return
	}
	if err := h.db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    loginResp.Token,
		"username": loginResp.User.Username,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to store credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "connected",
		"url":      req.URL,
		"username": loginResp.User.Username,
	})
}

// GetServer returns the current connection status.
func (h *RemoteHandler) GetServer(c *gin.Context) {
	var cfg store.Config
	h.db.First(&cfg)
	var creds store.Credentials
	h.db.First(&creds)

	status := "disconnected"
	if cfg.ServerURL != "" && creds.Token != "" {
		status = "connected"
	}
	c.JSON(http.StatusOK, gin.H{
		"status":   status,
		"url":      cfg.ServerURL,
		"username": creds.Username,
	})
}

// DisconnectServer clears stored credentials.
func (h *RemoteHandler) DisconnectServer(c *gin.Context) {
	if err := h.db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", "").Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to clear server config"})
		return
	}
	if err := h.db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    "",
		"username": "",
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to clear credentials"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
}

// ConnectViaProxy authenticates with a remote server using an IdToken cookie
// forwarded from the browser (via jupyter-server-proxy). This enables zero-click
// auto-connection when running inside a Nebari JupyterLab pod.
//
// Flow:
//  1. Browser has IdToken cookie (set by Envoy Gateway after Keycloak OIDC login)
//  2. jupyter-server-proxy forwards ALL headers (including cookies) to local Nebi
//  3. This endpoint reads the IdToken cookie from the incoming request
//  4. Forwards it to the remote Nebi server's /auth/session endpoint
//  5. Remote Nebi validates the cookie and returns a Nebi JWT
//  6. We store the JWT locally for all future remote API calls
func (h *RemoteHandler) ConnectViaProxy(c *gin.Context) {
	// Get remote URL from request body or NEBI_REMOTE_URL env var
	var req struct {
		URL string `json:"url"`
	}
	_ = c.ShouldBindJSON(&req)
	remoteURL := req.URL
	if remoteURL == "" {
		remoteURL = os.Getenv("NEBI_REMOTE_URL")
	}
	if remoteURL == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no remote URL provided and NEBI_REMOTE_URL not set"})
		return
	}
	remoteURL = strings.TrimRight(remoteURL, "/")

	// Find the IdToken cookie from the incoming request (forwarded by jupyter-server-proxy)
	var idTokenCookie string
	for _, cookie := range c.Request.Cookies() {
		if strings.HasPrefix(cookie.Name, "IdToken") {
			idTokenCookie = cookie.Name + "=" + cookie.Value
			break
		}
	}
	if idTokenCookie == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "no IdToken cookie found in request (is Keycloak auth configured?)"})
		return
	}

	// Call the remote Nebi server's /auth/session endpoint with the IdToken cookie
	sessionURL := remoteURL + "/api/v1/auth/session"
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), "GET", sessionURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to build request: %v", err)})
		return
	}
	httpReq.Header.Set("Cookie", idTokenCookie)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("failed to reach remote server: %v", err)})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("remote auth failed (status %d): %s", resp.StatusCode, string(body))})
		return
	}

	var loginResp struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &loginResp); err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("failed to parse remote response: %v", err)})
		return
	}

	// Store URL and credentials
	if err := h.db.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", remoteURL).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to store server URL"})
		return
	}
	if err := h.db.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
		"token":    loginResp.Token,
		"username": loginResp.User.Username,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to store credentials"})
		return
	}

	slog.Info("Auto-connected to remote Nebi via proxy cookie",
		"url", remoteURL,
		"username", loginResp.User.Username,
	)

	c.JSON(http.StatusOK, gin.H{
		"status":   "connected",
		"url":      remoteURL,
		"username": loginResp.User.Username,
	})
}

// GetAutoConnectConfig returns the auto-connect configuration.
// The frontend calls this on load to determine if it should auto-connect.
func (h *RemoteHandler) GetAutoConnectConfig(c *gin.Context) {
	remoteURL := os.Getenv("NEBI_REMOTE_URL")

	// Check current connection status
	var cfg store.Config
	h.db.First(&cfg)
	var creds store.Credentials
	h.db.First(&creds)

	alreadyConnected := cfg.ServerURL != "" && creds.Token != ""

	c.JSON(http.StatusOK, gin.H{
		"remote_url":        remoteURL,
		"auto_connect":      remoteURL != "",
		"already_connected": alreadyConnected,
	})
}

// ListWorkspaces proxies workspace listing to the remote server.
func (h *RemoteHandler) ListWorkspaces(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	workspaces, err := client.ListWorkspaces(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, workspaces)
}

// GetWorkspace proxies a single workspace fetch to the remote server.
func (h *RemoteHandler) GetWorkspace(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	ws, err := client.GetWorkspace(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, ws)
}

// CreateWorkspace proxies workspace creation to the remote server.
func (h *RemoteHandler) CreateWorkspace(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	var req cliclient.CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	ws, err := client.CreateWorkspace(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusCreated, ws)
}

// DeleteWorkspace proxies workspace deletion to the remote server.
func (h *RemoteHandler) DeleteWorkspace(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	if err := client.DeleteWorkspace(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListVersions proxies version listing for a remote workspace.
func (h *RemoteHandler) ListVersions(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	versions, err := client.GetWorkspaceVersions(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, versions)
}

// ListTags proxies tag listing for a remote workspace.
func (h *RemoteHandler) ListTags(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	tags, err := client.GetWorkspaceTags(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, tags)
}

// GetPixiToml proxies pixi.toml fetch for a remote workspace.
// Returns JSON {"content": "..."} for uniform frontend consumption.
func (h *RemoteHandler) GetPixiToml(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	var result struct {
		Content string `json:"content"`
	}
	if _, err := client.Get(c.Request.Context(), "/workspaces/"+id+"/pixi-toml", &result); err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetVersionPixiToml proxies version-specific pixi.toml fetch.
// Returns JSON {"content": "..."} — the upstream returns plain text but we
// wrap it in JSON for uniform frontend consumption.
func (h *RemoteHandler) GetVersionPixiToml(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	version := c.Param("version")
	versionNum, err := strconv.ParseInt(version, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid version number"})
		return
	}
	content, err := client.GetVersionPixiToml(c.Request.Context(), id, int32(versionNum))
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// GetVersionPixiLock proxies version-specific pixi.lock fetch.
// Returns JSON {"content": "..."} — see GetVersionPixiToml for rationale.
func (h *RemoteHandler) GetVersionPixiLock(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	version := c.Param("version")
	versionNum, err := strconv.ParseInt(version, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid version number"})
		return
	}
	content, err := client.GetVersionPixiLock(c.Request.Context(), id, int32(versionNum))
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// PushVersion proxies version push to the remote server.
func (h *RemoteHandler) PushVersion(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	var req cliclient.PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	resp, err := client.PushVersion(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

// ListRegistries proxies registry listing to the remote server.
func (h *RemoteHandler) ListRegistries(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	registries, err := client.ListRegistriesPublic(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, registries)
}

// ListJobs proxies job listing to the remote server.
func (h *RemoteHandler) ListJobs(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	jobs, err := client.ListJobs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

// ListAdminUsers proxies admin user listing to the remote server.
func (h *RemoteHandler) ListAdminUsers(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	users, err := client.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, users)
}

// ListAdminRegistries proxies admin registry listing to the remote server.
func (h *RemoteHandler) ListAdminRegistries(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	registries, err := client.ListRegistriesAdmin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, registries)
}

// ListAdminAuditLogs proxies admin audit log listing to the remote server.
func (h *RemoteHandler) ListAdminAuditLogs(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	userID := c.Query("user_id")
	action := c.Query("action")
	logs, err := client.ListAuditLogs(c.Request.Context(), userID, action)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// GetAdminDashboardStats proxies admin dashboard stats to the remote server.
func (h *RemoteHandler) GetAdminDashboardStats(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	stats, err := client.GetDashboardStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, stats)
}
