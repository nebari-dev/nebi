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

// ConnectServer godoc
// @Summary Connect to remote server
// @Description Authenticate with a remote Nebi server using username/password and store credentials locally
// @Tags remote
// @Accept json
// @Produce json
// @Param request body object true "Connection request with url, username, and password"
// @Success 200 {object} map[string]string "status, url, username"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /remote/connect [post]
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

// GetServer godoc
// @Summary Get remote server status
// @Description Returns the current remote server connection status, URL, and username
// @Tags remote
// @Produce json
// @Success 200 {object} map[string]string "status, url, username"
// @Router /remote/server [get]
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

// DisconnectServer godoc
// @Summary Disconnect from remote server
// @Description Clear stored remote server URL and credentials
// @Tags remote
// @Produce json
// @Success 200 {object} map[string]string "status"
// @Failure 500 {object} ErrorResponse
// @Router /remote/server [delete]
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

// ConnectViaProxy godoc
// @Summary Connect to remote server via proxy cookie
// @Description Authenticate with a remote Nebi server using an IdToken cookie forwarded by jupyter-server-proxy. Enables zero-click auto-connection for JupyterLab pods behind Envoy/Keycloak OIDC.
// @Tags remote
// @Accept json
// @Produce json
// @Param request body object false "Optional request with url (falls back to NEBI_REMOTE_URL env var)"
// @Success 200 {object} map[string]string "status, url, username"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "No IdToken cookie found"
// @Failure 500 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse "Remote server unreachable or auth failed"
// @Router /remote/connect-via-proxy [post]
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

// GetAutoConnectConfig godoc
// @Summary Get auto-connect configuration
// @Description Returns whether NEBI_REMOTE_URL is configured and whether the server is already connected. Used by the frontend on load to trigger auto-connection.
// @Tags remote
// @Produce json
// @Success 200 {object} map[string]interface{} "remote_url, auto_connect, already_connected"
// @Router /remote/auto-connect-config [get]
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

// ListWorkspaces godoc
// @Summary List remote workspaces
// @Description Proxy workspace listing to the connected remote server
// @Tags remote
// @Produce json
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces [get]
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

// GetWorkspace godoc
// @Summary Get remote workspace
// @Description Proxy a single workspace fetch to the connected remote server
// @Tags remote
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id} [get]
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

// CreateWorkspace godoc
// @Summary Create remote workspace
// @Description Proxy workspace creation to the connected remote server
// @Tags remote
// @Accept json
// @Produce json
// @Param request body cliclient.CreateWorkspaceRequest true "Workspace creation request"
// @Success 201 {object} object
// @Failure 400 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces [post]
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

// DeleteWorkspace godoc
// @Summary Delete remote workspace
// @Description Proxy workspace deletion to the connected remote server
// @Tags remote
// @Param id path string true "Workspace ID"
// @Success 204
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id} [delete]
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

// ListVersions godoc
// @Summary List remote workspace versions
// @Description Proxy version listing for a workspace on the connected remote server
// @Tags remote
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id}/versions [get]
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

// ListTags godoc
// @Summary List remote workspace tags
// @Description Proxy tag listing for a workspace on the connected remote server
// @Tags remote
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id}/tags [get]
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

// GetPixiToml godoc
// @Summary Get remote workspace pixi.toml
// @Description Proxy pixi.toml fetch for a workspace on the connected remote server
// @Tags remote
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]string "content"
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id}/pixi-toml [get]
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

// GetVersionPixiToml godoc
// @Summary Get remote workspace version pixi.toml
// @Description Proxy version-specific pixi.toml fetch from the connected remote server
// @Tags remote
// @Produce json
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {object} map[string]string "content"
// @Failure 400 {object} ErrorResponse "Invalid version number"
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id}/versions/{version}/pixi-toml [get]
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

// GetVersionPixiLock godoc
// @Summary Get remote workspace version pixi.lock
// @Description Proxy version-specific pixi.lock fetch from the connected remote server
// @Tags remote
// @Produce json
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {object} map[string]string "content"
// @Failure 400 {object} ErrorResponse "Invalid version number"
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id}/versions/{version}/pixi-lock [get]
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

// PushVersion godoc
// @Summary Push version to remote workspace
// @Description Proxy version push to a workspace on the connected remote server
// @Tags remote
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body cliclient.PushRequest true "Push request"
// @Success 201 {object} object
// @Failure 400 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/workspaces/{id}/push [post]
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

// ListRegistries godoc
// @Summary List remote registries
// @Description Proxy registry listing to the connected remote server
// @Tags remote
// @Produce json
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/registries [get]
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

// ListJobs godoc
// @Summary List remote jobs
// @Description Proxy job listing to the connected remote server
// @Tags remote
// @Produce json
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/jobs [get]
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

// ListAdminUsers godoc
// @Summary List remote admin users
// @Description Proxy admin user listing to the connected remote server
// @Tags remote
// @Produce json
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/admin/users [get]
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

// ListAdminRegistries godoc
// @Summary List remote admin registries
// @Description Proxy admin registry listing to the connected remote server
// @Tags remote
// @Produce json
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/admin/registries [get]
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

// ListAdminAuditLogs godoc
// @Summary List remote admin audit logs
// @Description Proxy admin audit log listing to the connected remote server
// @Tags remote
// @Produce json
// @Param user_id query string false "Filter by user ID"
// @Param action query string false "Filter by action"
// @Success 200 {array} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/admin/audit-logs [get]
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

// GetAdminDashboardStats godoc
// @Summary Get remote admin dashboard stats
// @Description Proxy admin dashboard statistics from the connected remote server
// @Tags remote
// @Produce json
// @Success 200 {object} object
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "Not connected to remote server"
// @Router /remote/admin/dashboard/stats [get]
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
