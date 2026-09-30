package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/oidcclient"
	"github.com/nebari-dev/nebi/internal/store"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// RemoteHandler proxies requests to a remote Nebi server.
// Used in local/desktop mode so the frontend can browse remote servers.
type RemoteHandler struct {
	db *gorm.DB

	mu      sync.Mutex
	pending *pendingConnect    // device authorization awaiting approval
	tokens  oauth2.TokenSource // cached so parallel requests share one refresh
}

// pendingConnect is a device authorization started by ConnectServer.
type pendingConnect struct {
	url       string
	clientID  string
	endpoints *oidcclient.Endpoints
	device    *oidcclient.DeviceAuthorization
	interval  int
	expiresAt time.Time
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
	if err := h.db.First(&creds).Error; err != nil || !creds.LoggedIn() {
		return nil, fmt.Errorf("not authenticated with remote server")
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tokens == nil {
		h.tokens = creds.TokenSource(context.Background(), func(tok *oauth2.Token) error {
			return store.SaveToken(h.db, tok)
		})
	}
	return cliclient.NewWithTokenSource(cfg.ServerURL, &resettingTokenSource{h: h, src: h.tokens}), nil
}

// resettingTokenSource drops the handler's cached token source when a
// refresh fails, so the next request starts again from the stored
// credentials (e.g. after the CLI refreshed and rotated them).
type resettingTokenSource struct {
	h   *RemoteHandler
	src oauth2.TokenSource
}

func (r *resettingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := r.src.Token()
	if err != nil {
		r.h.mu.Lock()
		if r.h.tokens == r.src {
			r.h.tokens = nil
		}
		r.h.mu.Unlock()
	}
	return tok, err
}

// notConnected returns 503 when no remote server is configured.
func (h *RemoteHandler) notConnected(c *gin.Context, err error) {
	c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
}

// ConnectServer starts connecting to a remote server. For a server with
// authentication disabled the connection is stored right away. Otherwise it
// starts an OAuth device authorization with the server's identity provider
// and returns the code the user approves in their browser; the frontend
// then calls PollConnect until the user has approved.
func (h *RemoteHandler) ConnectServer(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}
	serverURL := strings.TrimRight(strings.TrimSpace(req.URL), "/")
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Server URL must start with http:// or https://"})
		return
	}

	ctx := c.Request.Context()
	authCfg, err := cliclient.NewWithoutAuth(serverURL).GetAuthConfig(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Failed to connect: %v", err)})
		return
	}

	h.mu.Lock()
	h.pending = nil
	h.mu.Unlock()

	switch authCfg.Type {
	case cliclient.AuthTypeNone:
		h.finishConnect(c, serverURL, &store.Credentials{})
	case cliclient.AuthTypeOIDC:
		endpoints, err := oidcclient.Discover(ctx, authCfg.IssuerURL)
		if err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Failed to connect: %v", err)})
			return
		}
		scopes := append(authCfg.Scopes, oidcclient.OfflineAccessScope)
		device, err := oidcclient.StartDeviceAuthorization(ctx, endpoints, authCfg.ClientID, scopes)
		if err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Failed to connect: %v", err)})
			return
		}
		expiresIn := device.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 600
		}
		h.mu.Lock()
		h.pending = &pendingConnect{
			url:       serverURL,
			clientID:  authCfg.ClientID,
			endpoints: endpoints,
			device:    device,
			interval:  device.Interval,
			expiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
		}
		h.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{
			"user_code":                 device.UserCode,
			"verification_uri":          device.VerificationURI,
			"verification_uri_complete": device.VerificationURIComplete,
			"expires_in":                expiresIn,
			"interval":                  device.Interval,
		})
	default:
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote server uses unsupported authentication %q", authCfg.Type)})
	}
}

// PollConnect checks once whether the user approved the pending device
// authorization, and stores the credentials when they did.
func (h *RemoteHandler) PollConnect(c *gin.Context) {
	h.mu.Lock()
	p := h.pending
	h.mu.Unlock()
	if p == nil {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No connection in progress"})
		return
	}
	if time.Now().After(p.expiresAt) {
		h.clearPending(p)
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: oidcclient.ErrExpired.Error()})
		return
	}

	tok, err := oidcclient.PollDeviceToken(c.Request.Context(), p.endpoints, p.clientID, p.device)
	switch {
	case errors.Is(err, oidcclient.ErrAuthorizationPending):
		c.JSON(http.StatusOK, gin.H{"status": "pending", "interval": p.interval})
		return
	case errors.Is(err, oidcclient.ErrSlowDown):
		h.mu.Lock()
		p.interval += 5
		interval := p.interval
		h.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"status": "pending", "interval": interval})
		return
	case errors.Is(err, oidcclient.ErrExpired), errors.Is(err, oidcclient.ErrAccessDenied):
		h.clearPending(p)
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	case err != nil:
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Failed to connect: %v", err)})
		return
	}

	creds := &store.Credentials{TokenURL: p.endpoints.Token, ClientID: p.clientID}
	creds.SetOAuthToken(tok)
	if h.clearPending(p) {
		h.finishConnect(c, p.url, creds)
		return
	}
	c.JSON(http.StatusConflict, ErrorResponse{Error: "Connection was restarted"})
}

// clearPending forgets p if it is still the pending connection, reporting
// whether it was.
func (h *RemoteHandler) clearPending(p *pendingConnect) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pending != p {
		return false
	}
	h.pending = nil
	return true
}

// finishConnect resolves the remote user with creds and stores the connection.
func (h *RemoteHandler) finishConnect(c *gin.Context, serverURL string, creds *store.Credentials) {
	me, err := cliclient.New(serverURL, creds.Token).GetCurrentUser(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Failed to connect: %v", err)})
		return
	}
	creds.ID = 1
	creds.Username = me.Username

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", serverURL).Error; err != nil {
			return err
		}
		return tx.Save(creds).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to store the server connection"})
		return
	}
	h.mu.Lock()
	h.tokens = nil
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"status":   "connected",
		"url":      serverURL,
		"username": me.Username,
	})
}

// GetServer returns the current connection status.
func (h *RemoteHandler) GetServer(c *gin.Context) {
	var cfg store.Config
	h.db.First(&cfg)
	var creds store.Credentials
	h.db.First(&creds)

	status := "disconnected"
	if cfg.ServerURL != "" && creds.LoggedIn() {
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
	if err := h.db.Save(&store.Credentials{ID: 1}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to clear credentials"})
		return
	}
	h.mu.Lock()
	h.pending = nil
	h.tokens = nil
	h.mu.Unlock()
	c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
}

// ListProjects proxies project listing to the remote server.
func (h *RemoteHandler) ListProjects(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	projects, err := client.ListProjects(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, projects)
}

// GetProject proxies a single project fetch to the remote server.
func (h *RemoteHandler) GetProject(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	project, err := client.GetProject(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, project)
}

// CreateProject proxies project creation to the remote server.
func (h *RemoteHandler) CreateProject(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	var req cliclient.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}
	project, err := client.CreateProject(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusCreated, project)
}

// DeleteProject proxies project deletion to the remote server.
func (h *RemoteHandler) DeleteProject(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	if err := client.DeleteProject(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListVersions proxies version listing for a remote project.
func (h *RemoteHandler) ListVersions(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	versions, err := client.GetProjectVersions(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, versions)
}

// ListTags proxies tag listing for a remote project.
func (h *RemoteHandler) ListTags(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	id := c.Param("id")
	tags, err := client.GetProjectTags(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, tags)
}

// GetPixiToml proxies pixi.toml fetch for a remote project.
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
	if _, err := client.Get(c.Request.Context(), "/projects/"+id+"/pixi-toml", &result); err != nil {
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
		handleBindError(c, err)
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

// CreateAdminRegistry proxies registry creation to the remote server.
func (h *RemoteHandler) CreateAdminRegistry(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	var req cliclient.CreateRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	registry, err := client.CreateRegistry(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusCreated, registry)
}

// UpdateAdminRegistry proxies registry updates to the remote server.
func (h *RemoteHandler) UpdateAdminRegistry(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	var req cliclient.UpdateRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	registry, err := client.UpdateRegistry(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.JSON(http.StatusOK, registry)
}

// DeleteAdminRegistry proxies registry deletion to the remote server.
func (h *RemoteHandler) DeleteAdminRegistry(c *gin.Context) {
	client, err := h.getClient()
	if err != nil {
		h.notConnected(c, err)
		return
	}
	if err := client.DeleteRegistry(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("Remote error: %v", err)})
		return
	}
	c.Status(http.StatusNoContent)
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
