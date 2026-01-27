// Package server provides the main server initialization and run logic.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nebari-dev/nebi/internal/api"
	"github.com/nebari-dev/nebi/internal/api/handlers"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/localserver"
	"github.com/nebari-dev/nebi/internal/logger"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/worker"

	"github.com/valkey-io/valkey-go"
	"gorm.io/gorm"
)

// Config holds the server configuration options.
type Config struct {
	Port    int    // Port to run the server on (0 = use config default)
	Mode    string // Run mode: server, worker, or both
	Version string // Version string to report
}

// Run starts the server with the given configuration and blocks until the context is canceled.
// The optional cancelFunc is called when the local server idle timeout expires.
func Run(ctx context.Context, cfg Config, cancelFunc ...context.CancelFunc) error {
	// Set version in handlers
	if cfg.Version != "" {
		handlers.Version = cfg.Version
	}

	// Load configuration
	appCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override port from CLI flag if provided
	if cfg.Port != 0 {
		appCfg.Server.Port = cfg.Port
	}

	// Initialize logger
	logger.Init(appCfg.Log.Format, appCfg.Log.Level)
	slog.Info("Starting Nebi server", "version", cfg.Version, "mode", appCfg.Server.Mode)

	// Initialize database
	database, err := db.New(appCfg.Database)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	slog.Info("Database initialized", "driver", appCfg.Database.Driver)

	// Run migrations
	if err := db.Migrate(database); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	slog.Info("Database migrations completed")

	// Create default admin user if configured
	if err := db.CreateDefaultAdmin(database); err != nil {
		return fmt.Errorf("failed to create default admin user: %w", err)
	}

	// Initialize job queue based on configuration
	jobQueue, err := createQueue(appCfg, database)
	if err != nil {
		return fmt.Errorf("failed to initialize job queue: %w", err)
	}
	defer jobQueue.Close()
	slog.Info("Job queue initialized", "type", appCfg.Queue.Type)

	// Get Valkey client for log streaming (if using Valkey queue)
	var valkeyClient valkey.Client
	if vq, ok := jobQueue.(*queue.ValkeyQueue); ok {
		valkeyClient = vq.GetClient()
		slog.Info("Valkey client available for log streaming")
	}

	// Initialize executor
	exec, err := executor.NewLocalExecutor(appCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize executor: %w", err)
	}
	slog.Info("Local executor initialized")

	// Initialize components based on run mode
	var w *worker.Worker
	var srv *http.Server
	var workerCancel context.CancelFunc

	mode := cfg.Mode
	if mode == "" {
		mode = "both"
	}

	runServer := mode == "server" || mode == "both"
	runWorker := mode == "worker" || mode == "both"

	if !runServer && !runWorker {
		return fmt.Errorf("invalid mode %q: valid modes are server, worker, both", mode)
	}

	slog.Info("Starting Nebi", "mode", mode)

	// Initialize and start worker if needed
	if runWorker {
		w = worker.New(database, jobQueue, exec, slog.Default(), valkeyClient)
		workerCtx, cancel := context.WithCancel(ctx)
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
		var broker *logstream.LogBroker
		if w != nil {
			broker = w.GetBroker()
		}

		var valkeyClientInterface interface{} = valkeyClient
		router := api.NewRouter(appCfg, database, jobQueue, exec, broker, valkeyClientInterface, slog.Default())

		addr := fmt.Sprintf(":%d", appCfg.Server.Port)

		var handler http.Handler = router

		// If running as a local server, add idle timeout to auto-shutdown.
		if os.Getenv("NEBI_LOCAL_TOKEN") != "" && len(cancelFunc) > 0 {
			idleTimer := localserver.NewIdleTimer(localserver.DefaultIdleTimeout, func() {
				slog.Info("Idle timeout reached, shutting down local server")
				cancelFunc[0]()
			})
			defer idleTimer.Stop()
			handler = localserver.IdleTimeoutMiddleware(idleTimer, router)
			slog.Info("Idle timeout enabled", "timeout", localserver.DefaultIdleTimeout)
		}

		srv = &http.Server{
			Addr:    addr,
			Handler: handler,
		}

		go func() {
			slog.Info("Server listening", "address", addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Server failed", "error", err)
			}
		}()
	}

	// Write server.state if running as a local server (NEBI_LOCAL_TOKEN is set).
	localToken := os.Getenv("NEBI_LOCAL_TOKEN")
	if localToken != "" && runServer {
		state := &localserver.ServerState{
			PID:       os.Getpid(),
			Port:      appCfg.Server.Port,
			Token:     localToken,
			StartedAt: time.Now(),
		}
		if err := localserver.WriteState(state); err != nil {
			slog.Error("Failed to write server state", "error", err)
		} else {
			slog.Info("Server state written", "port", appCfg.Server.Port)
		}
		// Clean up state file on shutdown.
		defer func() {
			if err := localserver.RemoveState(); err != nil {
				slog.Error("Failed to remove server state", "error", err)
			}
		}()
	}

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("Shutting down...")

	// Stop worker if running
	if workerCancel != nil {
		workerCancel()
		slog.Info("Worker stopped")
	}

	// Shutdown server if running
	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server forced to shutdown: %w", err)
		}
		slog.Info("Server stopped")
	}

	slog.Info("Nebi exited")
	return nil
}

// RunWithSignalHandling starts the server and handles OS signals for graceful shutdown.
func RunWithSignalHandling(cfg Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Run server in goroutine, passing cancel so idle timeout can trigger shutdown.
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, cfg, cancel)
	}()

	// Wait for signal or error
	select {
	case sig := <-quit:
		slog.Info("Received signal", "signal", sig)
		cancel()
		// Wait for server to finish
		return <-errCh
	case err := <-errCh:
		return err
	}
}

// createQueue creates a queue based on configuration.
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
