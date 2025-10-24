package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aktech/darb/internal/api/handlers"
	"github.com/aktech/darb/internal/auth"
	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/executor"
	"github.com/aktech/darb/internal/queue"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// NewRouter creates and configures the Gin router
func NewRouter(cfg *config.Config, db *gorm.DB, q queue.Queue, exec executor.Executor) *gin.Engine {
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
		protected.GET("/environments/:id", envHandler.GetEnvironment)
		protected.DELETE("/environments/:id", envHandler.DeleteEnvironment)

		// Package endpoints
		protected.GET("/environments/:id/packages", envHandler.ListPackages)
		protected.POST("/environments/:id/packages", envHandler.InstallPackages)
		protected.DELETE("/environments/:id/packages/:package", envHandler.RemovePackages)

		// Environment configuration endpoints
		protected.GET("/environments/:id/pixi-toml", envHandler.GetPixiToml)

		// Job endpoints
		protected.GET("/jobs", jobHandler.ListJobs)
		protected.GET("/jobs/:id", jobHandler.GetJob)

		// Template endpoints (placeholder)
		protected.GET("/templates", handlers.NotImplemented)
		protected.POST("/templates", handlers.NotImplemented)

		// Admin endpoints (placeholder)
		admin := protected.Group("/admin")
		{
			admin.GET("/users", handlers.NotImplemented)
			admin.POST("/users", handlers.NotImplemented)
			admin.GET("/roles", handlers.NotImplemented)
			admin.POST("/permissions", handlers.NotImplemented)
			admin.GET("/audit-logs", handlers.NotImplemented)
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
