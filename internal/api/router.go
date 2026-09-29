package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/api/handlers"
	"github.com/nebari-dev/nebi/internal/api/middleware"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/config"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/netguard"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/nebari-dev/nebi/internal/web"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// NewRouter creates and configures the Gin router. ctx bounds background
// work the router starts (OIDC provider discovery retries).
func NewRouter(ctx context.Context, cfg *config.Config, db *gorm.DB, q *queue.MemoryQueue, exec executor.Executor, logBroker *logstream.LogBroker, logger *slog.Logger) *gin.Engine {
	// Initialize RBAC enforcer and provider.
	// In local mode the admin and project RBAC checks are unconditionally
	// skipped (see RequireAdmin / RequireProjectAccess middleware), so
	// there is no need to re-initialise the global casbin enforcer — and
	// doing so would clobber the enforcer that was already set up by a
	// concurrently-running team-mode server (relevant in tests).
	if !cfg.IsLocalMode() {
		if err := rbac.InitEnforcer(db, logger); err != nil {
			logger.Error("Failed to initialize RBAC", "error", err)
			panic(err)
		}
	}
	rbacProvider := rbac.NewDefaultProvider()

	// Set Gin mode
	if cfg.Server.Mode == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	localMode := cfg.IsLocalMode()
	basePath := cfg.Server.BasePath

	// Set handler-level mode for /version endpoint
	if localMode {
		handlers.Mode = "local"
	} else {
		handlers.Mode = "team"
	}

	router := gin.New()
	limitCfg := cfg.Limits

	// Middleware
	router.Use(gin.Recovery())
	router.Use(middleware.MaxRequestBodyBytes(limitCfg.RequestBodyBytes))
	router.Use(loggingMiddleware())
	router.Use(securityHeadersMiddleware(localMode, cfg.Server.AllowedOriginsList(), identityProviderOrigin(cfg)))
	router.Use(corsMiddleware(localMode, cfg.Server.AllowedOriginsList()))

	// Initialize the authenticator. Local mode and team mode with auth.type
	// "none" run every request as the well-known local user; team mode with
	// "oidc" accepts access tokens issued by the configured provider.
	var authenticator auth.Authenticator
	authConfig := handlers.AuthConfigResponse{Type: config.AuthTypeNone}
	if localMode || cfg.Auth.Type == config.AuthTypeNone {
		localAuth, err := auth.NewLocalAuthenticator(db)
		if err != nil {
			logger.Error("Failed to initialize local authenticator", "error", err)
			panic(err)
		}
		authenticator = localAuth
		if localMode {
			logger.Info("Running in local mode, authentication bypassed", "user", auth.LocalUsername())
		} else {
			// Admin checks still run in team mode, so the implicit user must be
			// an admin to reach the admin API.
			if err := rbacProvider.MakeAdmin(localAuth.User().ID); err != nil {
				logger.Error("Failed to grant admin to the unauthenticated user", "error", err)
				panic(err)
			}
			logger.Warn("Authentication is disabled (auth.type=none): every request runs as an admin",
				"user", auth.LocalUsername())
		}
	} else {
		oidcAuth := auth.NewOIDCAuthenticator(auth.OIDCConfig{
			IssuerURL:    cfg.Auth.OIDCIssuerURL,
			DiscoveryURL: cfg.Auth.OIDCDiscoveryURL,
			ClientID:     cfg.Auth.OIDCClientID,
			AdminGroups:  cfg.Auth.OIDCAdminGroupsList(),
		}, db, rbacProvider)
		oidcAuth.DiscoverInBackground(ctx, logger)
		authenticator = oidcAuth
		authConfig = handlers.AuthConfigResponse{
			Type:      config.AuthTypeOIDC,
			IssuerURL: cfg.Auth.OIDCIssuerURL,
			ClientID:  cfg.Auth.OIDCClientID,
			Scopes:    cfg.Auth.OIDCScopesList(),
		}
	}

	// Base group for all routes (supports reverse proxy path prefix)
	base := router.Group(basePath)

	// Public routes
	public := base.Group("/api/v1")
	{
		public.GET("/health", handlers.HealthCheck)
		public.GET("/version", handlers.GetVersion)
		public.GET("/auth/config", handlers.AuthConfig(authConfig))
	}

	// Derive encryption key for credential encryption at rest
	encKey, err := nebicrypto.DeriveKey(cfg.Auth.JWTSecret)
	if err != nil {
		logger.Error("Failed to derive encryption key", "error", err)
		panic(err)
	}

	// Initialize services and handlers
	svc := service.New(db, q, exec, localMode, encKey, rbacProvider, limitCfg)
	adminSvc := service.NewAdminService(db, rbacProvider, limitCfg)
	groupSvc := service.NewGroupService(db, rbacProvider) // INTERMEDIATE: old signature; final is NewGroupService(db)
	registrySvc := service.NewRegistryService(db, encKey, localMode, rbacProvider)
	jobSvc := service.NewJobService(db, localMode)

	projectHandler := handlers.NewProjectHandler(svc)
	groupHandler := handlers.NewGroupHandler(groupSvc)
	jobHandler := handlers.NewJobHandler(jobSvc, logBroker)

	// Protected routes (require authentication)
	protected := base.Group("/api/v1")
	protected.Use(authenticator.Middleware())
	{
		// User info
		protected.GET("/auth/me", handlers.GetCurrentUser)
		protected.GET("/groups/me", groupHandler.MyGroups)

		// Project endpoints
		protected.GET("/projects", projectHandler.ListProjects)
		protected.POST("/projects", projectHandler.CreateProject)

		// Per-project operations with RBAC permission checks
		project := protected.Group("/projects/:id")
		{
			// Read operations (require read permission)
			project.GET("", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.GetProject)
			project.GET("/packages", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.ListPackages)
			project.GET("/pixi-toml", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.GetPixiToml)
			project.GET("/collaborators", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.ListCollaborators)

			// Version operations (read permission)
			project.GET("/versions", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.ListVersions)
			project.GET("/versions/:version", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.GetVersion)
			project.GET("/versions/:version/pixi-lock", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.DownloadLockFile)
			project.GET("/versions/:version/pixi-toml", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.DownloadManifestFile)

			// Write operations (require write permission)
			project.PUT("/pixi-toml", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.SavePixiToml)
			project.DELETE("", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.DeleteProject)
			project.POST("/packages", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.InstallPackages)
			project.POST("/solve", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.SolveProject)
			project.POST("/install", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.InstallProject)
			project.POST("/uninstall", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.UninstallProject)
			project.DELETE("/packages/:package", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.RemovePackages)
			project.POST("/rollback", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.RollbackToVersion)

			// Sharing operations (owner only - checked in handler)
			project.POST("/share", projectHandler.ShareProject)
			project.DELETE("/share/:user_id", projectHandler.UnshareProject)
			project.POST("/share-group", projectHandler.ShareProjectWithGroup)
			project.DELETE("/share-group/:group_id", projectHandler.UnshareProjectWithGroup)

			// Tags (read permission)
			project.GET("/tags", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.ListTags)

			// Push and publish operations (require write permission)
			project.POST("/push", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.PushVersion)
			project.POST("/publish", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.PublishProject)
			project.GET("/publications", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.ListPublications)
			project.PATCH("/publications/:pubId", middleware.RequireProjectAccess("write", localMode, rbacProvider), projectHandler.UpdatePublication)
			project.GET("/publish-defaults", middleware.RequireProjectAccess("read", localMode, rbacProvider), projectHandler.GetPublishDefaults)
		}

		// Job endpoints
		protected.GET("/jobs", jobHandler.ListJobs)
		protected.GET("/jobs/:id", jobHandler.GetJob)
		protected.GET("/jobs/:id/logs/stream", jobHandler.StreamJobLogs)

		// Template endpoints (placeholder)
		protected.GET("/templates", handlers.NotImplemented)
		protected.POST("/templates", handlers.NotImplemented)

		// OCI Registry endpoints (for users to view available registries)
		registryHandler := handlers.NewRegistryHandler(registrySvc, adminSvc)
		protected.GET("/registries", registryHandler.ListPublicRegistries)

		// Registry browse & import endpoints (for all authenticated users)
		browseHandler := handlers.NewRegistryBrowseHandler(registrySvc, svc)
		protected.GET("/registries/:id/repositories", browseHandler.ListRepositories)
		protected.GET("/registries/:id/tags", browseHandler.ListTags)
		protected.POST("/registries/:id/import", browseHandler.ImportEnvironment)

		// Admin endpoints (require admin role)
		adminHandler := handlers.NewAdminHandler(adminSvc)
		admin := protected.Group("/admin")
		admin.Use(middleware.RequireAdmin(localMode, rbacProvider))
		{
			// Users (provisioned from the identity provider; read-only)
			admin.GET("/users", adminHandler.ListUsers)
			admin.GET("/users/:id", adminHandler.GetUser)
			admin.GET("/users/:id/groups", adminHandler.ListUserGroups)

			// Role management
			admin.GET("/roles", adminHandler.ListRoles)

			// Permission management
			admin.GET("/permissions", adminHandler.ListPermissions)
			admin.POST("/permissions", adminHandler.GrantPermission)
			admin.DELETE("/permissions/:id", adminHandler.RevokePermission)

			// Audit logs
			admin.GET("/audit-logs", adminHandler.ListAuditLogs)

			// Dashboard stats
			admin.GET("/dashboard/stats", adminHandler.GetDashboardStats)
			admin.GET("/resource-metrics", adminHandler.GetResourceMetrics)

			// OCI Registry management
			admin.GET("/registries", registryHandler.ListRegistries)
			admin.POST("/registries", registryHandler.CreateRegistry)
			admin.GET("/registries/:id", registryHandler.GetRegistry)
			admin.PUT("/registries/:id", registryHandler.UpdateRegistry)
			admin.DELETE("/registries/:id", registryHandler.DeleteRegistry)

			// Groups (synced from the identity provider; read-only)
			admin.GET("/groups", groupHandler.ListGroups)
			admin.GET("/groups/:id", groupHandler.GetGroup)
			admin.GET("/groups/:id/members", groupHandler.ListMembers)
			admin.POST("/registries/:id/grant-group", registryHandler.GrantRegistryToGroup)
			admin.DELETE("/registries/:id/grant-group/:group_id", registryHandler.RevokeRegistryFromGroup)
		}

		// Remote proxy endpoints (local mode only)
		if localMode {
			remoteHandler := handlers.NewRemoteHandler(db)
			remote := protected.Group("/remote")
			{
				remote.POST("/connect", remoteHandler.ConnectServer)
				remote.GET("/server", remoteHandler.GetServer)
				remote.DELETE("/server", remoteHandler.DisconnectServer)
				remote.GET("/projects", remoteHandler.ListProjects)
				remote.GET("/projects/:id", remoteHandler.GetProject)
				remote.POST("/projects", remoteHandler.CreateProject)
				remote.DELETE("/projects/:id", remoteHandler.DeleteProject)
				remote.GET("/projects/:id/versions", remoteHandler.ListVersions)
				remote.GET("/projects/:id/tags", remoteHandler.ListTags)
				remote.GET("/projects/:id/pixi-toml", remoteHandler.GetPixiToml)
				remote.GET("/projects/:id/versions/:version/pixi-toml", remoteHandler.GetVersionPixiToml)
				remote.GET("/projects/:id/versions/:version/pixi-lock", remoteHandler.GetVersionPixiLock)
				remote.POST("/projects/:id/push", remoteHandler.PushVersion)
				remote.GET("/registries", remoteHandler.ListRegistries)
				remote.GET("/jobs", remoteHandler.ListJobs)

				// Admin proxies (for view mode toggle in admin pages)
				remoteAdmin := remote.Group("/admin")
				// /remote routes are local-mode only, so this local admin check is
				// symbolic consistency. The remote server still enforces whether
				// the stored remote token has admin access.
				remoteAdmin.Use(middleware.RequireAdmin(localMode, rbacProvider))
				{
					remoteAdmin.GET("/users", remoteHandler.ListAdminUsers)
					remoteAdmin.GET("/registries", remoteHandler.ListAdminRegistries)
					remoteAdmin.POST("/registries", remoteHandler.CreateAdminRegistry)
					remoteAdmin.PUT("/registries/:id", remoteHandler.UpdateAdminRegistry)
					remoteAdmin.DELETE("/registries/:id", remoteHandler.DeleteAdminRegistry)
					remoteAdmin.GET("/audit-logs", remoteHandler.ListAdminAuditLogs)
					remoteAdmin.GET("/dashboard/stats", remoteHandler.GetAdminDashboardStats)
				}
			}
		}
	}

	// Swagger documentation
	base.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Serve embedded frontend
	embedFS, err := web.GetFileSystem()
	if err != nil {
		logger.Warn("Failed to load embedded frontend, frontend will not be served", "error", err)
	} else {
		runtimeBrandingConfigPath := resolveBrandingConfigPath()

		// SPA fallback - serve files from embedded filesystem for all non-API, non-docs routes
		router.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path

			// Strip base path prefix to get the relative path
			relPath := path
			if basePath != "" {
				if !strings.HasPrefix(path, basePath) {
					c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
					return
				}
				relPath = strings.TrimPrefix(path, basePath)
				if relPath == "" {
					relPath = "/"
				}
			}

			// Runtime branding config override:
			// if a Helm-mounted file exists on disk, serve it instead of embedded assets.
			if relPath == "/public/config.json" {
				content, err := os.ReadFile(runtimeBrandingConfigPath)
				if err == nil {
					c.Data(http.StatusOK, "application/json", content)
					return
				}
				if !os.IsNotExist(err) {
					logger.Warn("Failed to read runtime branding config", "path", runtimeBrandingConfigPath, "error", err)
				}
			}

			// Don't serve HTML for API calls or docs
			if strings.HasPrefix(relPath, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				return
			}
			if strings.HasPrefix(relPath, "/docs") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				return
			}

			// Remove leading slash for embedded FS
			fsPath := strings.TrimPrefix(relPath, "/")
			if fsPath == "" {
				fsPath = "index.html"
			}

			// Try to open the file in the embedded FS
			file, err := embedFS.Open(fsPath)
			if err != nil {
				// File doesn't exist, serve index.html for SPA routing
				fsPath = "index.html"
				file, err = embedFS.Open(fsPath)
				if err != nil {
					c.String(http.StatusInternalServerError, "Error loading frontend")
					return
				}
			}
			defer file.Close()

			// Read file content
			content, err := io.ReadAll(file)
			if err != nil {
				c.String(http.StatusInternalServerError, "Error reading file")
				return
			}

			// Set content type based on file extension
			contentType := "text/plain"
			if strings.HasSuffix(fsPath, ".html") {
				contentType = "text/html; charset=utf-8"
			} else if strings.HasSuffix(fsPath, ".js") {
				contentType = "application/javascript"
			} else if strings.HasSuffix(fsPath, ".css") {
				contentType = "text/css"
			} else if strings.HasSuffix(fsPath, ".json") {
				contentType = "application/json"
			} else if strings.HasSuffix(fsPath, ".svg") {
				contentType = "image/svg+xml"
			} else if strings.HasSuffix(fsPath, ".png") {
				contentType = "image/png"
			} else if strings.HasSuffix(fsPath, ".jpg") || strings.HasSuffix(fsPath, ".jpeg") {
				contentType = "image/jpeg"
			} else if strings.HasSuffix(fsPath, ".woff2") {
				contentType = "font/woff2"
			} else if strings.HasSuffix(fsPath, ".woff") {
				contentType = "font/woff"
			} else if strings.HasSuffix(fsPath, ".ttf") {
				contentType = "font/ttf"
			}

			// Rewrite absolute url(/...) references in bundled CSS (e.g. self-hosted
			// fonts) to include the base path. Without this they resolve against the
			// domain root and 404 when Nebi is served under a path prefix (proxy).
			if strings.HasSuffix(fsPath, ".css") && basePath != "" {
				content = []byte(strings.ReplaceAll(string(content), `url(/`, `url(`+basePath+`/`))
			}

			// For index.html, inject CSP nonce metadata and rewrite asset URLs
			if fsPath == "index.html" {
				html := string(content)
				nonceAttr := ""
				if styleNonce := c.GetString(cspStyleNonceKey); styleNonce != "" {
					nonceMeta := fmt.Sprintf(`<meta name="csp-style-nonce" content="%s" />`, styleNonce)
					html = strings.Replace(html, "<head>", "<head>\n    "+nonceMeta, 1)
				}
				if basePath != "" {
					if scriptNonce := c.GetString(cspScriptNonceKey); scriptNonce != "" {
						nonceAttr = fmt.Sprintf(` nonce="%s"`, scriptNonce)
					}
					// Inject base path script tag into <head>
					injection := fmt.Sprintf(`<script%s>window.__NEBI_BASE_PATH__=%q;</script>`, nonceAttr, basePath)
					html = strings.Replace(html, "<head>", "<head>\n    "+injection, 1)
					// Rewrite absolute asset paths to include base path
					html = strings.ReplaceAll(html, `href="/`, `href="`+basePath+`/`)
					html = strings.ReplaceAll(html, `src="/`, `src="`+basePath+`/`)
				}
				content = []byte(html)
			}

			c.Data(http.StatusOK, contentType, content)
		})

		logger.Info("Embedded frontend loaded and will be served")
	}

	slog.Info("API router initialized", "mode", cfg.Server.Mode, "app_mode", cfg.Mode)
	return router
}

func resolveBrandingConfigPath() string {
	path := strings.TrimSpace(os.Getenv("NEBI_BRANDING_CONFIG_PATH"))
	if path == "" {
		return "/app/public/config.json"
	}
	return path
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		slog.Info("HTTP request",
			"method", method,
			"path", path,
			"status", status,
			"latency", latency.String(),
			"ip", c.ClientIP(),
		)
	}
}

const (
	cspScriptNonceKey = "cspScriptNonce"
	cspStyleNonceKey  = "cspStyleNonce"
)

// identityProviderOrigin returns the origin of the OIDC issuer the web UI
// talks to directly (discovery, token and refresh requests), or "" when
// authentication is not delegated to an identity provider.
func identityProviderOrigin(cfg *config.Config) string {
	if cfg.IsLocalMode() || cfg.Auth.Type != config.AuthTypeOIDC {
		return ""
	}
	u, err := url.Parse(cfg.Auth.OIDCIssuerURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func securityHeadersMiddleware(localMode bool, allowedOrigins []string, idpOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scriptNonce, err := newCSPNonce()
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		styleNonce, err := newCSPNonce()
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		c.Set(cspScriptNonceKey, scriptNonce)
		c.Set(cspStyleNonceKey, styleNonce)
		headers := c.Writer.Header()
		headers.Set("Content-Security-Policy", contentSecurityPolicy(scriptNonce, styleNonce, localMode, allowedOrigins, idpOrigin))
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")
		headers.Set("X-Content-Type-Options", "nosniff")
		// Legacy fallback only: browsers that support CSP frame-ancestors
		// ignore this header, and it cannot express the configured-origins
		// allowlist, so SAMEORIGIN is the closest safe value.
		headers.Set("X-Frame-Options", "SAMEORIGIN")
		if isHTTPSRequest(c) {
			headers.Set("Strict-Transport-Security", "max-age=31536000")
		}

		c.Next()
	}
}

func contentSecurityPolicy(scriptNonce string, styleNonce string, localMode bool, allowedOrigins []string, idpOrigin string) string {
	connectSrc := "connect-src 'self'"
	if idpOrigin != "" {
		connectSrc += " " + idpOrigin
	}
	if localMode {
		connectSrc = "connect-src 'self' http://localhost:* http://127.0.0.1:* https://localhost:* https://127.0.0.1:*"
	}

	// 'self' covers reverse proxies that serve nebi and the framing page
	// from one origin (e.g. jupyter-server-proxy under JupyterHub);
	// server.allowed_origins covers frames served from a different origin.
	frameAncestors := "frame-ancestors 'self'"
	if len(allowedOrigins) > 0 {
		frameAncestors += " " + strings.Join(allowedOrigins, " ")
	}

	directives := []string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		frameAncestors,
		"script-src 'self' 'nonce-" + scriptNonce + "'",
		"style-src 'self' 'nonce-" + styleNonce + "'",
		"style-src-elem 'self' 'nonce-" + styleNonce + "'",
		"style-src-attr 'none'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		connectSrc,
		"manifest-src 'self'",
		"form-action 'self'",
	}
	return strings.Join(directives, "; ")
}

func newCSPNonce() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(nonce[:]), nil
}

func isHTTPSRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

// corsMiddleware adds CORS headers.
//
// The API is reached with a bearer Authorization header (CLI and the
// same-origin SPA), never with cross-origin cookies, so we do not set
// Access-Control-Allow-Credentials. A credentialed response is required by the
// CORS spec to name an explicit origin, and combining it with the "*" wildcard
// is invalid: browsers reject any such response, which in turn blocks the SPA's
// <script type="module" crossorigin> bundle from loading.
//
// In local mode the API is used only by local UIs, so instead of a wildcard
// the allowed origin is echoed only for those: local UI origins (see
// netguard.IsLocalUIOrigin — loopback http(s) origins like the SPA served by
// this process or the Vite dev server, the desktop webview's "wails://..."
// on macOS/Linux, and the opaque "null" origin some webviews produce), and
// any operator-configured server.allowed_origins (a reverse proxy such as
// jupyter-server-proxy, whose public origin browsers send on CORS-mode
// requests like Vite's crossorigin module bundles). netguard.Middleware
// enforces the same origin rules on the network listener, so a webview
// talking to it directly (wails dev with VITE_API_URL set) is admitted too.
func corsMiddleware(localMode bool, allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if localMode {
			origin := c.Request.Header.Get("Origin")
			if netguard.IsLocalUIOrigin(origin) || netguard.OriginAllowed(origin, allowedOrigins) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Vary", "Origin")
			}
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		// Prevent WebView (WKWebView) from caching API responses,
		// which would break polling-based UI updates in the desktop app.
		c.Writer.Header().Set("Cache-Control", "no-store")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
