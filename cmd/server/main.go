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

	"github.com/openteams-ai/darb/internal/api"
	"github.com/openteams-ai/darb/internal/config"
	"github.com/openteams-ai/darb/internal/db"
	"github.com/openteams-ai/darb/internal/executor"
	"github.com/openteams-ai/darb/internal/logger"
	"github.com/openteams-ai/darb/internal/logstream"
	"github.com/openteams-ai/darb/internal/queue"
	"github.com/openteams-ai/darb/internal/worker"

	_ "github.com/openteams-ai/darb/docs" // Load swagger docs

	"github.com/openteams-ai/darb/internal/api/handlers"
	"github.com/valkey-io/valkey-go"
	"gorm.io/gorm"
)

// Version is set via ldflags at build time
var Version = "dev"

// @title Darb API
// @version 1.0
// @description Multi-User Environment Management System API
// @host localhost:8460
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	// Define CLI flags
	port := flag.Int("port", 0, "Port to run the server on (overrides config)")
	mode := flag.String("mode", "both", "Run mode: server (API only), worker (worker only), or both")
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

	// Initialize job queue based on configuration
	jobQueue, err := createQueue(cfg, database)
	if err != nil {
		slog.Error("Failed to initialize job queue", "error", err)
		os.Exit(1)
	}
	defer jobQueue.Close()
	slog.Info("Job queue initialized", "type", cfg.Queue.Type)

	// Get Valkey client for log streaming (if using Valkey queue)
	var valkeyClient valkey.Client
	if vq, ok := jobQueue.(*queue.ValkeyQueue); ok {
		valkeyClient = vq.GetClient()
		slog.Info("Valkey client available for log streaming")
	}

	// Initialize executor
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		slog.Error("Failed to initialize executor", "error", err)
		os.Exit(1)
	}
	slog.Info("Local executor initialized")

	// Initialize components based on run mode
	var w *worker.Worker
	var srv *http.Server
	var workerCancel context.CancelFunc

	runServer := *mode == "server" || *mode == "both"
	runWorker := *mode == "worker" || *mode == "both"

	if !runServer && !runWorker {
		slog.Error("Invalid mode", "mode", *mode, "valid_modes", "server, worker, both")
		os.Exit(1)
	}

	slog.Info("Starting Darb", "mode", *mode)

	// Initialize and start worker if needed
	if runWorker {
		// Create worker with optional Valkey client (nil for local mode, non-nil for distributed mode)
		w = worker.New(database, jobQueue, exec, slog.Default(), valkeyClient)
		workerCtx, cancel := context.WithCancel(context.Background())
		workerCancel = cancel

		go func() {
			if err := w.Start(workerCtx); err != nil && err != context.Canceled {
				slog.Error("Worker failed", "error", err)
			}
		}()
		slog.Info("Worker started")
	}

	// Initialize and start API server if needed
	if runServer {
		// Get broker for log streaming (nil if worker not running)
		var broker *logstream.LogBroker
		if w != nil {
			broker = w.GetBroker()
		}

		// Initialize API router (pass valkeyClient as interface{} for compatibility)
		var valkeyClientInterface interface{} = valkeyClient
		router := api.NewRouter(cfg, database, jobQueue, exec, broker, valkeyClientInterface, slog.Default())

		// Create HTTP server
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		srv = &http.Server{
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
	}

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down...")

	// Stop worker if running
	if workerCancel != nil {
		workerCancel()
		slog.Info("Worker stopped")
	}

	// Shutdown server if running
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Server forced to shutdown", "error", err)
			os.Exit(1)
		}
		slog.Info("Server stopped")
	}

	slog.Info("Darb exited")
}

// createQueue creates a queue based on configuration
func createQueue(cfg *config.Config, database *gorm.DB) (queue.Queue, error) {
	switch cfg.Queue.Type {
	case "memory":
		return queue.NewMemoryQueue(100), nil
	case "valkey":
		if cfg.Queue.ValkeyAddr == "" {
			return nil, fmt.Errorf("valkey address is required when queue type is valkey")
		}
		return queue.NewValkeyQueue(cfg.Queue.ValkeyAddr, database)
	default:
		return nil, fmt.Errorf("unsupported queue type: %s (supported: memory, valkey)", cfg.Queue.Type)
	}
}
