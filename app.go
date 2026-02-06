package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nebari-dev/nebi/internal/api"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/nebari-dev/nebi/internal/worker"
	"gorm.io/gorm"
)

// getAppDataDir returns the appropriate data directory for the desktop app
func getAppDataDir() (string, error) {
	var baseDir string

	switch runtime.GOOS {
	case "darwin":
		// macOS: ~/Library/Application Support/Nebi
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(homeDir, "Library", "Application Support", "nebi")
	case "windows":
		// Windows: %APPDATA%\Nebi
		baseDir = filepath.Join(os.Getenv("APPDATA"), "nebi")
	default:
		// Linux: ~/.local/share/nebi
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(homeDir, ".local", "share", "nebi")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}

	return baseDir, nil
}

// App struct holds application state for the desktop app
type App struct {
	ctx    context.Context
	db     *gorm.DB
	config *config.Config
	server *http.Server
}

// NewApp creates a new App instance
func NewApp() *App {
	return &App{}
}

// logToFile writes a message to the debug log file
func logToFile(msg string) {
	f, err := os.OpenFile("/tmp/nebi-startup.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("15:04:05"), msg)
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logToFile("=== Startup called ===")

	// Set default admin credentials for desktop app (first-run setup)
	if os.Getenv("ADMIN_USERNAME") == "" {
		os.Setenv("ADMIN_USERNAME", "admin")
	}
	if os.Getenv("ADMIN_PASSWORD") == "" {
		os.Setenv("ADMIN_PASSWORD", "admin")
	}
	if os.Getenv("ADMIN_EMAIL") == "" {
		os.Setenv("ADMIN_EMAIL", "admin@localhost")
	}

	// Set database path to user's Application Support directory for desktop app
	dataDir, err := getAppDataDir()
	if err != nil {
		logToFile(fmt.Sprintf("Error getting app data dir: %v", err))
		return
	}
	dbPath := fmt.Sprintf("%s/nebi.db", dataDir)
	os.Setenv("NEBI_DATABASE_DSN", dbPath)
	logToFile(fmt.Sprintf("Using database: %s", dbPath))

	// Set storage directory to app data dir (fixes read-only file system error)
	storageDir := filepath.Join(dataDir, "workspaces")
	os.Setenv("NEBI_STORAGE_DIR", storageDir)
	logToFile(fmt.Sprintf("Using storage: %s", storageDir))

	// Ensure desktop app runs in local mode
	os.Setenv("NEBI_MODE", "local")

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logToFile(fmt.Sprintf("Error loading config: %v", err))
		return
	}
	a.config = cfg
	logToFile("Config loaded")

	// Connect to database
	database, err := db.New(cfg.Database)
	if err != nil {
		logToFile(fmt.Sprintf("Error connecting to database: %v", err))
		return
	}
	a.db = database
	logToFile("Database connected")

	// Run migrations
	if err := db.Migrate(database); err != nil {
		logToFile(fmt.Sprintf("Error running migrations: %v", err))
		return
	}
	// Desktop app is always local mode — migrate store tables
	if err := store.MigrateServerDB(database); err != nil {
		logToFile(fmt.Sprintf("Error migrating store tables: %v", err))
		return
	}
	logToFile("Migrations complete")

	// Create default admin user if none exists
	if err := db.CreateDefaultAdmin(database); err != nil {
		logToFile(fmt.Sprintf("Warning creating admin: %v", err))
	}
	logToFile("Admin user checked")

	// Start embedded API server for the frontend
	logToFile("Starting embedded server goroutine...")
	go a.startEmbeddedServer(cfg, database)
	logToFile("Startup complete")
}

// startEmbeddedServer starts the HTTP API server for the frontend to use
func (a *App) startEmbeddedServer(cfg *config.Config, database *gorm.DB) {
	logToFile("startEmbeddedServer: entering")

	// Initialize job queue (memory queue for desktop app)
	jobQueue := queue.NewMemoryQueue(100)
	logToFile("startEmbeddedServer: queue created")

	// Initialize executor
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		logToFile(fmt.Sprintf("startEmbeddedServer: executor error: %v", err))
		return
	}
	logToFile("startEmbeddedServer: executor initialized")

	// Create worker
	w := worker.New(database, jobQueue, exec, slog.Default(), nil)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	_ = workerCancel // Keep reference to avoid unused warning
	logToFile("startEmbeddedServer: worker created")

	go func() {
		logToFile("startEmbeddedServer: worker starting...")
		if err := w.Start(workerCtx); err != nil && err != context.Canceled {
			logToFile(fmt.Sprintf("startEmbeddedServer: worker error: %v", err))
		}
	}()

	// Initialize API router
	logToFile("startEmbeddedServer: initializing router...")
	router := api.NewRouter(cfg, database, jobQueue, exec, w.GetBroker(), nil, slog.Default())
	logToFile("startEmbeddedServer: router initialized")

	// Create HTTP server on port 8460
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	a.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	logToFile(fmt.Sprintf("startEmbeddedServer: starting server on %s", addr))
	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logToFile(fmt.Sprintf("startEmbeddedServer: server error: %v", err))
	}
	logToFile("startEmbeddedServer: server stopped")
}

// WailsWorkspace represents a simplified workspace for the Wails frontend
type WailsWorkspace struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	PackageManager string `json:"packageManager"`
	CreatedAt      string `json:"createdAt"`
}

// ListWorkspaces returns all workspaces
func (a *App) ListWorkspaces() ([]WailsWorkspace, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	var workspaces []models.Workspace
	if err := a.db.Order("created_at DESC").Find(&workspaces).Error; err != nil {
		return nil, err
	}

	result := make([]WailsWorkspace, len(workspaces))
	for i, ws := range workspaces {
		result[i] = WailsWorkspace{
			ID:             ws.ID.String(),
			Name:           ws.Name,
			Status:         string(ws.Status),
			PackageManager: ws.PackageManager,
			CreatedAt:      ws.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	return result, nil
}

// CreateWorkspace creates a new workspace
func (a *App) CreateWorkspace(name string, pixiToml string) (*WailsWorkspace, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ws := models.Workspace{
		Name:           name,
		Status:         models.WsStatusPending,
		PackageManager: "pixi",
	}

	if err := a.db.Create(&ws).Error; err != nil {
		return nil, err
	}

	return &WailsWorkspace{
		ID:             ws.ID.String(),
		Name:           ws.Name,
		Status:         string(ws.Status),
		PackageManager: ws.PackageManager,
		CreatedAt:      ws.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

// DeleteWorkspace deletes a workspace by ID
func (a *App) DeleteWorkspace(id string) error {
	if a.db == nil {
		return fmt.Errorf("database not connected")
	}

	return a.db.Where("id = ?", id).Delete(&models.Workspace{}).Error
}

// GetWorkspace gets a single workspace by ID
func (a *App) GetWorkspace(id string) (*WailsWorkspace, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	var ws models.Workspace
	if err := a.db.Where("id = ?", id).First(&ws).Error; err != nil {
		return nil, err
	}

	return &WailsWorkspace{
		ID:             ws.ID.String(),
		Name:           ws.Name,
		Status:         string(ws.Status),
		PackageManager: ws.PackageManager,
		CreatedAt:      ws.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}
