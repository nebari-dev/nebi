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
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
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

	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware())
	router.Use(corsMiddleware())

	// Initialize authenticator
	var authenticator auth.Authenticator
	var oidcAuth *auth.OIDCAuthenticator
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

	// Public routes
	public := router.Group("/api/v1")
	{
		public.GET("/health", handlers.HealthCheck)
		public.GET("/version", handlers.GetVersion)
		public.POST("/auth/login", handlers.Login(authenticator))

		// OIDC routes (if enabled)
		if oidcAuth != nil {
			public.GET("/auth/oidc/login", handlers.OIDCLogin(oidcAuth))
			public.GET("/auth/oidc/callback", handlers.OIDCCallback(oidcAuth))
		}
	}

	// Initialize handlers
	envHandler := handlers.NewEnvironmentHandler(db, q, exec)
	jobHandler := handlers.NewJobHandler(db, logBroker, valkeyClient)

	// Protected routes (require authentication)
	protected := router.Group("/api/v1")
	protected.Use(authenticator.Middleware())
	{
		// User info
		protected.GET("/auth/me", handlers.GetCurrentUser(authenticator))

		// Environment endpoints
		protected.GET("/environments", envHandler.ListEnvironments)
		protected.POST("/environments", envHandler.CreateEnvironment)

		// Per-environment operations with RBAC permission checks
		env := protected.Group("/environments/:id")
		{
			// Read operations (require read permission)
			env.GET("", middleware.RequireEnvironmentAccess("read"), envHandler.GetEnvironment)
			env.GET("/packages", middleware.RequireEnvironmentAccess("read"), envHandler.ListPackages)
			env.GET("/pixi-toml", middleware.RequireEnvironmentAccess("read"), envHandler.GetPixiToml)
			env.GET("/collaborators", middleware.RequireEnvironmentAccess("read"), envHandler.ListCollaborators)

			// Version operations (read permission)
			env.GET("/versions", middleware.RequireEnvironmentAccess("read"), envHandler.ListVersions)
			env.GET("/versions/:version", middleware.RequireEnvironmentAccess("read"), envHandler.GetVersion)
			env.GET("/versions/:version/pixi-lock", middleware.RequireEnvironmentAccess("read"), envHandler.DownloadLockFile)
			env.GET("/versions/:version/pixi-toml", middleware.RequireEnvironmentAccess("read"), envHandler.DownloadManifestFile)

			// Write operations (require write permission)
			env.DELETE("", middleware.RequireEnvironmentAccess("write"), envHandler.DeleteEnvironment)
			env.POST("/packages", middleware.RequireEnvironmentAccess("write"), envHandler.InstallPackages)
			env.DELETE("/packages/:package", middleware.RequireEnvironmentAccess("write"), envHandler.RemovePackages)
			env.POST("/rollback", middleware.RequireEnvironmentAccess("write"), envHandler.RollbackToVersion)

			// Sharing operations (owner only - checked in handler)
			env.POST("/share", envHandler.ShareEnvironment)
			env.DELETE("/share/:user_id", envHandler.UnshareEnvironment)

			// Tags (read permission)
			env.GET("/tags", middleware.RequireEnvironmentAccess("read"), envHandler.ListTags)

			// Push and publish operations (require write permission)
			env.POST("/push", middleware.RequireEnvironmentAccess("write"), envHandler.PushVersion)
			env.POST("/publish", middleware.RequireEnvironmentAccess("write"), envHandler.PublishEnvironment)
			env.GET("/publications", middleware.RequireEnvironmentAccess("read"), envHandler.ListPublications)
		}

		// Job endpoints
		protected.GET("/jobs", jobHandler.ListJobs)
		protected.GET("/jobs/:id", jobHandler.GetJob)
		protected.GET("/jobs/:id/logs/stream", jobHandler.StreamJobLogs)

		// Template endpoints (placeholder)
		protected.GET("/templates", handlers.NotImplemented)
		protected.POST("/templates", handlers.NotImplemented)

		// OCI Registry endpoints (for users to view available registries)
		registryHandler := handlers.NewRegistryHandler(db)
		protected.GET("/registries", registryHandler.ListPublicRegistries)

		// Admin endpoints (require admin role)
		adminHandler := handlers.NewAdminHandler(db)
		admin := protected.Group("/admin")
		admin.Use(middleware.RequireAdmin())
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

	slog.Info("API router initialized", "mode", cfg.Server.Mode)
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

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
