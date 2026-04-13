package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/nebari-dev/nebi/internal/web"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// NewRouter creates and configures the Gin router
func NewRouter(cfg *config.Config, db *gorm.DB, q queue.Queue, exec executor.Executor, logBroker *logstream.LogBroker, valkeyClient interface{}, logger *slog.Logger) *gin.Engine {
	// Initialize RBAC enforcer
	if err := rbac.InitEnforcer(db, logger); err != nil {
		logger.Error("Failed to initialize RBAC", "error", err)
		panic(err)
	}

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

	// Middleware
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware())
	router.Use(corsMiddleware())

	// Initialize authenticator based on mode
	var authenticator auth.Authenticator
	var oidcAuth *auth.OIDCAuthenticator
	// Session check endpoint needs a BasicAuthenticator for JWT generation.
	sessionBasicAuth := auth.NewBasicAuthenticator(db, cfg.Auth.JWTSecret)

	if localMode {
		localAuth, err := auth.NewLocalAuthenticator(db)
		if err != nil {
			logger.Error("Failed to initialize local authenticator", "error", err)
			panic(err)
		}
		authenticator = localAuth
		logger.Info("Running in local mode — authentication bypassed", "user", auth.LocalUsername())
	} else {
		if cfg.Auth.Type == "basic" {
			basicAuth := auth.NewBasicAuthenticator(db, cfg.Auth.JWTSecret)
			basicAuth.SetProxyAdminGroups(cfg.Auth.ProxyAdminGroups)
			authenticator = basicAuth
		}

		// Initialize OIDC if configured
		if cfg.Auth.OIDCIssuerURL != "" && cfg.Auth.OIDCClientID != "" {
			oidcCfg := auth.OIDCConfig{
				IssuerURL:    cfg.Auth.OIDCIssuerURL,
				ClientID:     cfg.Auth.OIDCClientID,
				ClientSecret: cfg.Auth.OIDCClientSecret,
				RedirectURL:  cfg.Auth.OIDCRedirectURL,
			}
			var err error
			// Use context.Background() for initialization
			oidcAuth, err = auth.NewOIDCAuthenticator(nil, oidcCfg, db, cfg.Auth.JWTSecret)
			if err != nil {
				logger.Error("Failed to initialize OIDC authenticator, will retry in background", "error", err)
				// Retry in background — the OIDC provider (e.g. Keycloak) may not
				// be ready yet at startup. Once it becomes reachable, wire the
				// verifier into the authenticators so proxy auth starts working.
				go retryOIDCInit(oidcCfg, db, cfg.Auth.JWTSecret, sessionBasicAuth, authenticator, logger)
			} else {
				logger.Info("OIDC authentication enabled", "issuer", cfg.Auth.OIDCIssuerURL)
			}
		}
	}

	// When OIDC is configured, logout requires redirecting to the gateway's
	// /logout path to clear OIDC cookies and terminate the Keycloak session.
	// Set this based on configuration, not initialization success, so the
	// frontend knows about the gateway even while OIDC retries in background.
	oidcConfigured := cfg.Auth.OIDCIssuerURL != "" && cfg.Auth.OIDCClientID != ""
	if oidcConfigured {
		handlers.LogoutURL = basePath + "/logout"
	}

	// Wire OIDC ID token verifier into authenticators that handle proxy cookies.
	if oidcAuth != nil {
		sessionBasicAuth.SetIDTokenVerifier(oidcAuth.Verifier())
		if ba, ok := authenticator.(*auth.BasicAuthenticator); ok {
			ba.SetIDTokenVerifier(oidcAuth.Verifier())
		}
	}

	// Base group for all routes (supports reverse proxy path prefix)
	base := router.Group(basePath)

	// Authorization code store for the gateway session redirect flow.
	// Codes are short-lived (30s) and single-use.
	authCodeStore := auth.NewAuthCodeStore()

	// Session redirect: exchanges an OIDC proxy IdToken cookie for a
	// single-use authorization code (RFC 6749 §4.1 pattern) and redirects
	// to /login?code=<code>. The frontend exchanges the code for a JWT via
	// POST /api/v1/auth/code/exchange. This path is outside /api/ so that
	// gateway proxies that strip cookies from public routes still forward
	// them here.
	base.GET("/auth/session", handlers.SessionRedirect(sessionBasicAuth, cfg.Auth.ProxyAdminGroups, basePath, authCodeStore))

	// Public routes
	public := base.Group("/api/v1")
	{
		public.GET("/health", handlers.HealthCheck)
		public.GET("/version", handlers.GetVersion)
		public.POST("/auth/login", handlers.Login(authenticator))

		// Session check: exchanges proxy IdToken cookie for a Nebi JWT (no auth middleware)
		public.GET("/auth/session", handlers.SessionCheck(sessionBasicAuth, cfg.Auth.ProxyAdminGroups))

		// Code exchange: frontend exchanges a single-use authorization code for a JWT.
		// The code was generated by GET /auth/session (the protected redirect endpoint).
		public.POST("/auth/code/exchange", handlers.CodeExchange(authCodeStore))

		// CLI login: device code flow for browser-based CLI authentication.
		cliCodeStore := auth.NewDeviceCodeStore()
		cliLoginHandler := handlers.CLILogin(sessionBasicAuth, cfg.Auth.ProxyAdminGroups, cliCodeStore)
		public.POST("/auth/cli-login/code", handlers.CLILoginCode(cliCodeStore))
		public.GET("/auth/cli-login", cliLoginHandler)
		public.POST("/auth/cli-login", cliLoginHandler)
		public.GET("/auth/cli-login/poll", handlers.CLILoginPoll(cliCodeStore))

		// Device flow: RFC 8628 configuration and token exchange for CLI.
		public.GET("/auth/device-config", handlers.DeviceConfig(cfg.Auth.OIDCIssuerURL, cfg.Auth.DeviceFlowClientID))
		public.POST("/auth/device-token", handlers.DeviceToken(sessionBasicAuth, cfg.Auth.ProxyAdminGroups))

		// OIDC routes (if enabled, team mode only)
		if oidcAuth != nil {
			public.GET("/auth/oidc/login", handlers.OIDCLogin(oidcAuth))
			public.GET("/auth/oidc/callback", handlers.OIDCCallback(oidcAuth, authCodeStore, basePath))
		}
	}

	// Derive encryption key for credential encryption at rest
	encKey, err := nebicrypto.DeriveKey(cfg.Auth.JWTSecret)
	if err != nil {
		logger.Error("Failed to derive encryption key", "error", err)
		panic(err)
	}

	// Initialize service and handlers
	svc := service.New(db, q, exec, localMode)
	wsHandler := handlers.NewWorkspaceHandler(svc, db, q, exec, localMode, encKey)
	jobHandler := handlers.NewJobHandler(db, logBroker, valkeyClient)

	// Protected routes (require authentication)
	protected := base.Group("/api/v1")
	protected.Use(authenticator.Middleware())
	{
		// User info
		protected.GET("/auth/me", handlers.GetCurrentUser(authenticator))

		// Workspace endpoints
		protected.GET("/workspaces", wsHandler.ListWorkspaces)
		protected.POST("/workspaces", wsHandler.CreateWorkspace)

		// Per-workspace operations with RBAC permission checks
		ws := protected.Group("/workspaces/:id")
		{
			// Read operations (require read permission)
			ws.GET("", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.GetWorkspace)
			ws.GET("/packages", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.ListPackages)
			ws.GET("/pixi-toml", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.GetPixiToml)
			ws.GET("/collaborators", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.ListCollaborators)

			// Version operations (read permission)
			ws.GET("/versions", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.ListVersions)
			ws.GET("/versions/:version", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.GetVersion)
			ws.GET("/versions/:version/pixi-lock", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.DownloadLockFile)
			ws.GET("/versions/:version/pixi-toml", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.DownloadManifestFile)

			// Write operations (require write permission)
			ws.PUT("/pixi-toml", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.SavePixiToml)
			ws.DELETE("", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.DeleteWorkspace)
			ws.POST("/packages", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.InstallPackages)
			ws.POST("/solve", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.SolveWorkspace)
			ws.DELETE("/packages/:package", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.RemovePackages)
			ws.POST("/rollback", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.RollbackToVersion)

			// Sharing operations (owner only - checked in handler)
			ws.POST("/share", wsHandler.ShareWorkspace)
			ws.DELETE("/share/:user_id", wsHandler.UnshareWorkspace)

			// Tags (read permission)
			ws.GET("/tags", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.ListTags)

			// Push and publish operations (require write permission)
			ws.POST("/push", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.PushVersion)
			ws.POST("/publish", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.PublishWorkspace)
			ws.GET("/publications", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.ListPublications)
			ws.PATCH("/publications/:pubId", middleware.RequireWorkspaceAccess("write", localMode), wsHandler.UpdatePublication)
			ws.GET("/publish-defaults", middleware.RequireWorkspaceAccess("read", localMode), wsHandler.GetPublishDefaults)
		}

		// Job endpoints
		protected.GET("/jobs", jobHandler.ListJobs)
		protected.GET("/jobs/:id", jobHandler.GetJob)
		protected.GET("/jobs/:id/logs/stream", jobHandler.StreamJobLogs)

		// Template endpoints (placeholder)
		protected.GET("/templates", handlers.NotImplemented)
		protected.POST("/templates", handlers.NotImplemented)

		// OCI Registry endpoints (for users to view available registries)
		registryHandler := handlers.NewRegistryHandler(db, encKey)
		protected.GET("/registries", registryHandler.ListPublicRegistries)

		// Registry browse & import endpoints (for all authenticated users)
		browseHandler := handlers.NewRegistryBrowseHandler(db, svc, encKey)
		protected.GET("/registries/:id/repositories", browseHandler.ListRepositories)
		protected.GET("/registries/:id/tags", browseHandler.ListTags)
		protected.POST("/registries/:id/import", browseHandler.ImportEnvironment)

		// Admin endpoints (require admin role)
		adminHandler := handlers.NewAdminHandler(db)
		admin := protected.Group("/admin")
		admin.Use(middleware.RequireAdmin(localMode))
		{
			// User management
			admin.GET("/users", adminHandler.ListUsers)
			admin.POST("/users", adminHandler.CreateUser)
			admin.GET("/users/:id", adminHandler.GetUser)
			admin.POST("/users/:id/toggle-admin", adminHandler.ToggleAdmin)
			admin.DELETE("/users/:id", adminHandler.DeleteUser)

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

			// OCI Registry management
			admin.GET("/registries", registryHandler.ListRegistries)
			admin.POST("/registries", registryHandler.CreateRegistry)
			admin.GET("/registries/:id", registryHandler.GetRegistry)
			admin.PUT("/registries/:id", registryHandler.UpdateRegistry)
			admin.DELETE("/registries/:id", registryHandler.DeleteRegistry)
		}

		// Remote proxy endpoints (local mode only)
		if localMode {
			remoteHandler := handlers.NewRemoteHandler(db)
			remote := protected.Group("/remote")
			{
				remote.POST("/connect", remoteHandler.ConnectServer)
				remote.GET("/server", remoteHandler.GetServer)
				remote.DELETE("/server", remoteHandler.DisconnectServer)
				remote.GET("/workspaces", remoteHandler.ListWorkspaces)
				remote.GET("/workspaces/:id", remoteHandler.GetWorkspace)
				remote.POST("/workspaces", remoteHandler.CreateWorkspace)
				remote.DELETE("/workspaces/:id", remoteHandler.DeleteWorkspace)
				remote.GET("/workspaces/:id/versions", remoteHandler.ListVersions)
				remote.GET("/workspaces/:id/tags", remoteHandler.ListTags)
				remote.GET("/workspaces/:id/pixi-toml", remoteHandler.GetPixiToml)
				remote.GET("/workspaces/:id/versions/:version/pixi-toml", remoteHandler.GetVersionPixiToml)
				remote.GET("/workspaces/:id/versions/:version/pixi-lock", remoteHandler.GetVersionPixiLock)
				remote.POST("/workspaces/:id/push", remoteHandler.PushVersion)
				remote.GET("/registries", remoteHandler.ListRegistries)
				remote.GET("/jobs", remoteHandler.ListJobs)

				// Admin proxies (for view mode toggle in admin pages)
				remote.GET("/admin/users", remoteHandler.ListAdminUsers)
				remote.GET("/admin/registries", remoteHandler.ListAdminRegistries)
				remote.GET("/admin/audit-logs", remoteHandler.ListAdminAuditLogs)
				remote.GET("/admin/dashboard/stats", remoteHandler.GetAdminDashboardStats)
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
			}

			// For index.html, inject base path and rewrite asset URLs
			if fsPath == "index.html" && basePath != "" {
				html := string(content)
				// Inject base path script tag into <head>
				injection := fmt.Sprintf(`<script>window.__NEBI_BASE_PATH__=%q;</script>`, basePath)
				html = strings.Replace(html, "<head>", "<head>\n    "+injection, 1)
				// Rewrite absolute asset paths to include base path
				html = strings.ReplaceAll(html, `href="/`, `href="`+basePath+`/`)
				html = strings.ReplaceAll(html, `src="/`, `src="`+basePath+`/`)
				content = []byte(html)
			}

			c.Data(http.StatusOK, contentType, content)
		})

		logger.Info("Embedded frontend loaded and will be served")
	}

	slog.Info("API router initialized", "mode", cfg.Server.Mode, "app_mode", cfg.Mode)
	return router
}

// retryOIDCInit retries OIDC provider discovery in the background until it
// succeeds. This handles the case where the OIDC provider (e.g. Keycloak) is
// not yet ready when Nebi starts. Once discovery succeeds, the ID token
// verifier is wired into the authenticators so proxy auth starts working.
func retryOIDCInit(cfg auth.OIDCConfig, db *gorm.DB, jwtSecret string,
	sessionAuth *auth.BasicAuthenticator, mainAuth auth.Authenticator, logger *slog.Logger) {
	for {
		time.Sleep(10 * time.Second)
		oa, err := auth.NewOIDCAuthenticator(nil, cfg, db, jwtSecret)
		if err != nil {
			logger.Warn("OIDC initialization retry failed, will try again", "error", err)
			continue
		}
		logger.Info("OIDC authentication enabled (after retry)", "issuer", cfg.IssuerURL)
		sessionAuth.SetIDTokenVerifier(oa.Verifier())
		if ba, ok := mainAuth.(*auth.BasicAuthenticator); ok {
			ba.SetIDTokenVerifier(oa.Verifier())
		}
		return
	}
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

// corsMiddleware adds CORS headers
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
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
