package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aktech/darb/internal/api"
	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/db"
	"github.com/aktech/darb/internal/executor"
	"github.com/aktech/darb/internal/logger"
	"github.com/aktech/darb/internal/queue"
	"github.com/aktech/darb/internal/worker"

	_ "github.com/aktech/darb/docs" // Load swagger docs

	"github.com/aktech/darb/internal/api/handlers"
)

// Version is set via ldflags at build time
var Version = "dev"

// @title Darb API
// @version 1.0
// @description Multi-User Environment Management System API
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	// Define CLI flags
	port := flag.Int("port", 0, "Port to run the server on (overrides config)")
	flag.Parse()

	// Set version in handlers
	handlers.Version = Version
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Override port from CLI flag if provided
	if *port != 0 {
		cfg.Server.Port = *port
	}

	// Initialize logger
	logger.Init(cfg.Log.Format, cfg.Log.Level)
	slog.Info("Starting Darb server", "version", Version, "mode", cfg.Server.Mode)

	// Initialize database
	database, err := db.New(cfg.Database)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	slog.Info("Database initialized", "driver", cfg.Database.Driver)

	// Run migrations
	if err := db.Migrate(database); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("Database migrations completed")

	// Create default admin user if configured
	if err := db.CreateDefaultAdmin(database); err != nil {
		slog.Error("Failed to create default admin user", "error", err)
		os.Exit(1)
	}

	// Initialize job queue
	jobQueue := queue.NewMemoryQueue(100)
	slog.Info("Job queue initialized", "type", cfg.Queue.Type)

	// Initialize executor
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		slog.Error("Failed to initialize executor", "error", err)
		os.Exit(1)
	}
	slog.Info("Local executor initialized")

	// Initialize and start worker
	w := worker.New(database, jobQueue, exec, slog.Default())
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	go func() {
		if err := w.Start(workerCtx); err != nil && err != context.Canceled {
			slog.Error("Worker failed", "error", err)
		}
	}()
	slog.Info("Worker started")

	// Initialize API router (pass worker's broker for log streaming)
	router := api.NewRouter(cfg, database, jobQueue, exec, w.GetBroker(), slog.Default())

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		slog.Info("Server listening", "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	// Stop worker
	workerCancel()
	slog.Info("Worker stopped")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("Server exited")
}
