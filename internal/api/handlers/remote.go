package handlers

import (
	"fmt"
	"net/http"
	"strconv"

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
