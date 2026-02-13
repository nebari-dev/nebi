package api

import (
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
			authenticator = auth.NewBasicAuthenticator(db, cfg.Auth.JWTSecret)
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
				logger.Error("Failed to initialize OIDC authenticator", "error", err)
			} else {
				logger.Info("OIDC authentication enabled", "issuer", cfg.Auth.OIDCIssuerURL)
			}
		}
	}

	// Public routes
	public := router.Group("/api/v1")
	{
		public.GET("/health", handlers.HealthCheck)
		public.GET("/version", handlers.GetVersion)
		public.POST("/auth/login", handlers.Login(authenticator))

		// OIDC routes (if enabled, team mode only)
		if oidcAuth != nil {
			public.GET("/auth/oidc/login", handlers.OIDCLogin(oidcAuth))
			public.GET("/auth/oidc/callback", handlers.OIDCCallback(oidcAuth))
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
	protected := router.Group("/api/v1")
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
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Serve embedded frontend
	embedFS, err := web.GetFileSystem()
	if err != nil {
		logger.Warn("Failed to load embedded frontend, frontend will not be served", "error", err)
	} else {
		// SPA fallback - serve files from embedded filesystem for all non-API, non-docs routes
		router.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path

			// Don't serve HTML for API calls or docs
			if strings.HasPrefix(path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				return
			}
			if strings.HasPrefix(path, "/docs") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				return
			}

			// Remove leading slash for embedded FS
			fsPath := strings.TrimPrefix(path, "/")
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

			c.Data(http.StatusOK, contentType, content)
		})

		logger.Info("Embedded frontend loaded and will be served")
	}

	slog.Info("API router initialized", "mode", cfg.Server.Mode, "app_mode", cfg.Mode)
	return router
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
