package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
)

// RemoteHandler handles remote server proxy operations.
type RemoteHandler struct {
	db *gorm.DB
}

// NewRemoteHandler creates a new RemoteHandler.
func NewRemoteHandler(db *gorm.DB) *RemoteHandler {
	return &RemoteHandler{db: db}
}

// ConnectRequest is the request body for connecting to a remote server.
type ConnectRequest struct {
	URL      string `json:"url" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ConnectServer authenticates with a remote Nebi server and stores the connection.
// POST /api/v1/remote/server
func (h *RemoteHandler) ConnectServer(c *gin.Context) {
	var req ConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Login to the remote server
	client := cliclient.NewWithoutAuth(req.URL)
	loginResp, err := client.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to authenticate with remote server: " + err.Error()})
		return
	}

	// Save URL to store_config singleton
	if err := h.db.Model(&store.Config{}).Where("id = 1").Update("server_url", req.URL).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save remote connection"})
		return
	}

	// Save token and username to store_credentials singleton
	if err := h.db.Model(&store.Credentials{}).Where("id = 1").Updates(map[string]interface{}{
		"token":    loginResp.Token,
		"username": req.Username,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save remote credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":      req.URL,
		"username": req.Username,
		"status":   "connected",
	})
}

// GetServer returns the current remote server connection status.
// GET /api/v1/remote/server
func (h *RemoteHandler) GetServer(c *gin.Context) {
	var cfg store.Config
	h.db.First(&cfg, 1)

	if cfg.ServerURL == "" {
		c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
		return
	}

	var creds store.Credentials
	h.db.First(&creds, 1)

	c.JSON(http.StatusOK, gin.H{
		"url":      cfg.ServerURL,
		"username": creds.Username,
		"status":   "connected",
	})
}

// DisconnectServer removes the remote server connection.
// DELETE /api/v1/remote/server
func (h *RemoteHandler) DisconnectServer(c *gin.Context) {
	h.db.Model(&store.Config{}).Where("id = 1").Update("server_url", "")
	h.db.Model(&store.Credentials{}).Where("id = 1").Updates(map[string]interface{}{
		"token":    "",
		"username": "",
	})
	c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
}

// getClient returns a cliclient configured for the stored remote server.
func (h *RemoteHandler) getClient() (*cliclient.Client, error) {
	var cfg store.Config
	if err := h.db.First(&cfg, 1).Error; err != nil {
		return nil, err
	}
	var creds store.Credentials
	if err := h.db.First(&creds, 1).Error; err != nil {
		return nil, err
	}
	return cliclient.New(cfg.ServerURL, creds.Token), nil
}

// ListWorkspaces proxies workspace listing from the remote server.
// GET /api/v1/remote/workspaces
func (h *RemoteHandler) ListWorkspaces(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	workspaces, err := client.ListWorkspaces(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to list remote workspaces: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, workspaces)
}

// GetWorkspace proxies a single workspace from the remote server.
// GET /api/v1/remote/workspaces/:id
func (h *RemoteHandler) GetWorkspace(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	ws, err := client.GetWorkspace(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to get remote workspace: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, ws)
}

// ListVersions proxies version listing for a remote workspace.
// GET /api/v1/remote/workspaces/:id/versions
func (h *RemoteHandler) ListVersions(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	versions, err := client.GetWorkspaceVersions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to list remote versions: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, versions)
}

// ListTags proxies tag listing for a remote workspace.
// GET /api/v1/remote/workspaces/:id/tags
func (h *RemoteHandler) ListTags(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	tags, err := client.GetWorkspaceTags(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to list remote tags: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, tags)
}

// GetPixiToml proxies the current pixi.toml for a remote workspace.
// GET /api/v1/remote/workspaces/:id/pixi-toml
func (h *RemoteHandler) GetPixiToml(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	// Use the latest version's pixi.toml
	versions, err := client.GetWorkspaceVersions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to get remote versions: " + err.Error()})
		return
	}

	if len(versions) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No versions found"})
		return
	}

	// Find the latest version
	var latest int32
	for _, v := range versions {
		if v.VersionNumber > latest {
			latest = v.VersionNumber
		}
	}

	content, err := client.GetVersionPixiToml(c.Request.Context(), c.Param("id"), latest)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to get remote pixi.toml: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": content})
}

// GetVersionPixiToml proxies the pixi.toml for a specific version of a remote workspace.
// GET /api/v1/remote/workspaces/:id/versions/:version/pixi-toml
func (h *RemoteHandler) GetVersionPixiToml(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	version, err := strconv.ParseInt(c.Param("version"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version number"})
		return
	}

	content, err := client.GetVersionPixiToml(c.Request.Context(), c.Param("id"), int32(version))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to get remote pixi.toml: " + err.Error()})
		return
	}

	c.String(http.StatusOK, content)
}

// GetVersionPixiLock proxies the pixi.lock for a specific version of a remote workspace.
// GET /api/v1/remote/workspaces/:id/versions/:version/pixi-lock
func (h *RemoteHandler) GetVersionPixiLock(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	version, err := strconv.ParseInt(c.Param("version"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version number"})
		return
	}

	content, err := client.GetVersionPixiLock(c.Request.Context(), c.Param("id"), int32(version))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to get remote pixi.lock: " + err.Error()})
		return
	}

	c.String(http.StatusOK, content)
}

// CreateWorkspace creates a workspace on the remote server.
// POST /api/v1/remote/workspaces
func (h *RemoteHandler) CreateWorkspace(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	var req cliclient.CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ws, err := client.CreateWorkspace(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to create remote workspace: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, ws)
}

// DeleteWorkspace deletes a workspace on the remote server.
// DELETE /api/v1/remote/workspaces/:id
func (h *RemoteHandler) DeleteWorkspace(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	if err := client.DeleteWorkspace(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to delete remote workspace: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// PushVersion pushes a new version to a remote workspace.
// POST /api/v1/remote/workspaces/:id/push
func (h *RemoteHandler) PushVersion(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not connected to a remote server"})
		return
	}

	var req cliclient.PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := client.PushVersion(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to push version: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
