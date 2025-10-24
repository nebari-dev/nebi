package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aktech/darb/internal/api/handlers"
	"github.com/aktech/darb/internal/api/middleware"
	"github.com/aktech/darb/internal/auth"
	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/executor"
	"github.com/aktech/darb/internal/queue"
	"github.com/aktech/darb/internal/rbac"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// NewRouter creates and configures the Gin router
func NewRouter(cfg *config.Config, db *gorm.DB, q queue.Queue, exec executor.Executor, logger *slog.Logger) *gin.Engine {
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
	if cfg.Auth.Type == "basic" {
		authenticator = auth.NewBasicAuthenticator(db, cfg.Auth.JWTSecret)
	}
	// TODO: Add OIDC support in future

	// Public routes
	public := router.Group("/api/v1")
	{
		public.GET("/health", handlers.HealthCheck)
		public.POST("/auth/login", handlers.Login(authenticator))
	}

	// Initialize handlers
	envHandler := handlers.NewEnvironmentHandler(db, q, exec)
	jobHandler := handlers.NewJobHandler(db)

	// Protected routes (require authentication)
	protected := router.Group("/api/v1")
	protected.Use(authenticator.Middleware())
	{
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

			// Write operations (require write permission)
			env.DELETE("", middleware.RequireEnvironmentAccess("write"), envHandler.DeleteEnvironment)
			env.POST("/packages", middleware.RequireEnvironmentAccess("write"), envHandler.InstallPackages)
			env.DELETE("/packages/:package", middleware.RequireEnvironmentAccess("write"), envHandler.RemovePackages)

			// Sharing operations (owner only - checked in handler)
			env.POST("/share", envHandler.ShareEnvironment)
			env.DELETE("/share/:user_id", envHandler.UnshareEnvironment)
		}

		// Job endpoints
		protected.GET("/jobs", jobHandler.ListJobs)
		protected.GET("/jobs/:id", jobHandler.GetJob)

		// Template endpoints (placeholder)
		protected.GET("/templates", handlers.NotImplemented)
		protected.POST("/templates", handlers.NotImplemented)

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
		}
	}

	// Swagger documentation
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
