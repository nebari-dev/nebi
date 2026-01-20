package desktop

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/openteams-ai/darb/internal/api"
	"github.com/openteams-ai/darb/internal/config"
	"github.com/openteams-ai/darb/internal/db"
	"github.com/openteams-ai/darb/internal/executor"
	"github.com/openteams-ai/darb/internal/logger"
	"github.com/openteams-ai/darb/internal/queue"
	"github.com/openteams-ai/darb/internal/worker"
	"gorm.io/gorm"
)

// Version is set via ldflags at build time
var Version = "dev"

// App represents the desktop application with embedded server
type App struct {
	ctx          context.Context
	config       *config.Config
	database     *gorm.DB
	jobQueue     queue.Queue
	executor     executor.Executor
	worker       *worker.Worker
	server       *http.Server
	workerCancel context.CancelFunc
	serverAddr   string
	mu           sync.Mutex
	started      bool
}

// NewApp creates a new desktop application instance
func NewApp() *App {
	return &App{}
}

// Startup is called when the Wails app starts
// It initializes all components and starts the embedded server
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize logging
	logger.Init("text", "info")
	slog.Info("Starting Darb Desktop", "version", Version)

	// Initialize components in a goroutine to avoid blocking the UI
	go func() {
		if err := a.initializeComponents(); err != nil {
			slog.Error("Failed to initialize components", "error", err)
			return
		}

		if err := a.startServer(); err != nil {
			slog.Error("Failed to start embedded server", "error", err)
			return
		}

		a.mu.Lock()
		a.started = true
		a.mu.Unlock()

		slog.Info("Darb Desktop started successfully", "address", a.serverAddr)
	}()
}

// initializeComponents sets up database, queue, executor, and worker
func (a *App) initializeComponents() error {
	// Load desktop-specific configuration
	cfg, err := NewDesktopConfig()
	if err != nil {
		return fmt.Errorf("failed to create desktop config: %w", err)
	}
	a.config = cfg

	// Initialize database
	database, err := db.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	a.database = database
	slog.Info("Database initialized", "driver", cfg.Database.Driver, "path", cfg.Database.DSN)

	// Run migrations
	if err := db.Migrate(database); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	slog.Info("Database migrations completed")

	// Create default admin user if configured via environment variables
	if err := db.CreateDefaultAdmin(database); err != nil {
		slog.Warn("Failed to create default admin user", "error", err)
		// Don't fail startup, user might already exist
	}

	// Check if we need to create a default desktop user
	if err := a.ensureDesktopUser(); err != nil {
		slog.Warn("Failed to ensure desktop user exists", "error", err)
	}

	// Initialize job queue (memory-based for desktop)
	a.jobQueue = queue.NewMemoryQueue(100)
	slog.Info("Job queue initialized", "type", "memory")

	// Initialize executor
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize executor: %w", err)
	}
	a.executor = exec
	slog.Info("Local executor initialized")

	// Initialize and start worker
	a.worker = worker.New(database, a.jobQueue, exec, slog.Default(), nil)
	workerCtx, cancel := context.WithCancel(context.Background())
	a.workerCancel = cancel

	go func() {
		if err := a.worker.Start(workerCtx); err != nil && err != context.Canceled {
			slog.Error("Worker failed", "error", err)
		}
	}()
	slog.Info("Worker started")

	return nil
}

// startServer starts the embedded HTTP server
func (a *App) startServer() error {
	// Get broker for log streaming
	broker := a.worker.GetBroker()

	// Initialize API router
	router := api.NewRouter(a.config, a.database, a.jobQueue, a.executor, broker, nil, slog.Default())

	// Bind to localhost only (security: not exposed to network)
	a.serverAddr = "127.0.0.1:8460"
	a.server = &http.Server{
		Addr:         a.serverAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("Server listening", "address", a.serverAddr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
		}
	}()

	return nil
}

// ensureDesktopUser creates a default desktop user if no users exist
func (a *App) ensureDesktopUser() error {
	// Check environment variables first
	username := os.Getenv("DARB_ADMIN_USERNAME")
	password := os.Getenv("DARB_ADMIN_PASSWORD")

	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "admin"
	}

	// Set environment variables so CreateDefaultAdmin picks them up
	os.Setenv("ADMIN_USERNAME", username)
	os.Setenv("ADMIN_PASSWORD", password)

	return db.CreateDefaultAdmin(a.database)
}

// Shutdown is called when the Wails app is closing
func (a *App) Shutdown(ctx context.Context) {
	slog.Info("Shutting down Darb Desktop...")

	a.mu.Lock()
	started := a.started
	a.mu.Unlock()

	if !started {
		return
	}

	// Stop worker
	if a.workerCancel != nil {
		a.workerCancel()
		slog.Info("Worker stopped")
	}

	// Shutdown server
	if a.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			slog.Error("Server forced to shutdown", "error", err)
		} else {
			slog.Info("Server stopped")
		}
	}

	// Close queue
	if a.jobQueue != nil {
		a.jobQueue.Close()
	}

	slog.Info("Darb Desktop shutdown complete")
}

// GetServerAddress returns the address of the embedded server
func (a *App) GetServerAddress() string {
	return "http://" + a.serverAddr
}

// IsReady returns true if the server is started and ready
func (a *App) IsReady() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.started
}

// GetVersion returns the application version
func (a *App) GetVersion() string {
	return Version
}

// GetDataDir returns the application data directory path
func (a *App) GetDataDir() string {
	dir, err := GetDataDir()
	if err != nil {
		return ""
	}
	return dir
}
