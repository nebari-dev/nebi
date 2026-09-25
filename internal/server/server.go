// Package server provides the main server initialization and run logic.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nebari-dev/nebi/internal/api"
	"github.com/nebari-dev/nebi/internal/api/handlers"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/config"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/logger"
	"github.com/nebari-dev/nebi/internal/netguard"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/nebari-dev/nebi/internal/worker"
)

// Config holds the server configuration options.
type Config struct {
	Host        string      // Bind host/IP (empty = config/default behavior)
	Port        int         // Port to run the server on (0 = use config default)
	RuntimeMode config.Mode // Runtime mode: team or local
	Version     string      // Version string to report
	Commit      string      // Git commit hash
}

// Run starts the server with the given configuration and blocks until the context is canceled.
func Run(ctx context.Context, cfg Config) error {
	// Set version in handlers
	if cfg.Version != "" {
		handlers.Version = cfg.Version
	}
	if cfg.Commit != "" {
		handlers.Commit = cfg.Commit
	}

	// Load configuration
	runtimeMode := cfg.RuntimeMode
	if runtimeMode == "" {
		runtimeMode = config.ModeTeam
	}
	appCfg, err := config.Load(config.WithMode(runtimeMode))
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override host/port from CLI flags if provided
	if strings.TrimSpace(cfg.Host) != "" {
		appCfg.Server.Host = strings.TrimSpace(cfg.Host)
	}
	if cfg.Port != 0 {
		appCfg.Server.Port = cfg.Port
	}
	appCfg.Server.Host = resolveBindHost(appCfg.IsLocalMode(), appCfg.Server.Host)

	// Initialize logger
	logger.Init(appCfg.Log.Format, appCfg.Log.Level)
	slog.Info("Starting Nebi server", "version", cfg.Version, "mode", appCfg.Server.Mode)
	if appCfg.Server.BasePath != "" {
		slog.Info("Base path configured", "base_path", appCfg.Server.BasePath)
	}

	// Initialize database
	database, err := db.New(appCfg.Database)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	slog.Info("Database initialized", "driver", appCfg.Database.Driver)

	// Run migrations
	if err := db.Migrate(database, appCfg.Registries.SeedDefault); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	if appCfg.IsLocalMode() {
		if err := store.MigrateServerDB(database); err != nil {
			return fmt.Errorf("failed to migrate store tables: %w", err)
		}

		// Auto-connect to remote server if configured via environment
		if remoteURL := os.Getenv("NEBI_REMOTE_URL"); remoteURL != "" {
			if authToken := os.Getenv("NEBI_AUTH_TOKEN"); authToken != "" {
				database.Model(&store.Config{}).Where("id = ?", 1).Update("server_url", remoteURL)
				database.Model(&store.Credentials{}).Where("id = ?", 1).Updates(map[string]any{
					"token": authToken,
				})
				slog.Info("Auto-connected to remote server", "url", remoteURL)
			}
		}
	}
	slog.Info("Database migrations completed")

	// Reconcile admin-provisioned registries from config.yaml into the DB.
	// Runs unconditionally so entries removed from config are cleaned up.
	if err := service.ReconcileConfigRegistries(database, appCfg.Registries.Entries); err != nil {
		return fmt.Errorf("failed to reconcile config registries: %w", err)
	}

	// Create default admin user if configured (team mode only)
	if !appCfg.IsLocalMode() {
		// Initialize RBAC early so CreateDefaultAdmin can grant admin role
		if err := rbac.InitEnforcer(database, slog.Default()); err != nil {
			return fmt.Errorf("failed to initialize RBAC: %w", err)
		}
		if err := db.CreateDefaultAdmin(database, rbac.NewDefaultProvider()); err != nil {
			return fmt.Errorf("failed to create default admin user: %w", err)
		}
	}

	// The API and worker share an in-process queue.
	jobQueue := queue.NewMemoryQueue(100)
	defer jobQueue.Close()

	// Initialize executor
	exec, err := executor.NewLocalExecutor(appCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize executor: %w", err)
	}
	slog.Info("Local executor initialized")

	limitCfg := appCfg.Limits

	// Initialize the services used by the in-process worker.
	workerEncKey, err := nebicrypto.DeriveKey(appCfg.Auth.JWTSecret)
	if err != nil {
		return fmt.Errorf("failed to derive encryption key: %w", err)
	}
	workerSvc := service.New(database, jobQueue, exec, appCfg.IsLocalMode(), workerEncKey, rbac.NewDefaultProvider(), limitCfg)
	workerJobSvc := service.NewJobService(database, appCfg.IsLocalMode())

	// Run jobs in the background while the HTTP API remains responsive.
	w := worker.New(jobQueue, exec, workerSvc, workerJobSvc, slog.Default(), limitCfg, appCfg.Worker.MaxParallelJobs)
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	go func() {
		if err := w.Start(workerCtx); err != nil && err != context.Canceled {
			slog.Error("Worker failed", "error", err)
		}
	}()

	router := api.NewRouter(appCfg, database, jobQueue, exec, w.GetBroker(), slog.Default())
	if !appCfg.IsLocalMode() {
		auth.StartAuthReconciliationMonitor(ctx, database, rbac.NewDefaultProvider(), slog.Default())
	}

	var handler http.Handler = router
	if appCfg.IsLocalMode() {
		// Local mode is a single-user, on-device setup: scope the
		// listener to clients on the local machine.
		allowAnyHost := !netguard.IsLoopbackHost(appCfg.Server.Host)
		if allowAnyHost {
			slog.Warn("Local mode is bound to a non-loopback interface; it is intended for local use only",
				"host", appCfg.Server.Host)
		}
		handler = netguard.Middleware(router, allowAnyHost, appCfg.Server.AllowedOriginsList())
	}

	addr := listenAddress(appCfg.Server.Host, appCfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: config.HTTPReadHeaderTimeout,
		ReadTimeout:       appCfg.Server.ReadTimeout(),
		WriteTimeout:      limitCfg.HTTPWriteTimeout(),
		IdleTimeout:       config.HTTPIdleTimeout,
		MaxHeaderBytes:    config.HTTPMaxHeaderBytes,
	}

	go func() {
		slog.Info("Server listening", "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
		}
	}()

	url := serverURL(appCfg.Server.Host, appCfg.Server.Port, appCfg.Server.BasePath)
	fmt.Printf("\n  \033[32m✔\033[0m Server running at \033[1;36m%s\033[0m\n\n", url)

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("Shutting down...")

	workerCancel()
	slog.Info("Worker stopped")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}
	slog.Info("Server stopped")

	slog.Info("Nebi exited")
	return nil
}

// resolveBindHost returns the effective bind host. Local mode is a
// single-user, on-device setup, so when no host is configured it binds
// loopback only; binding more widely requires an explicit host (config,
// env, or flag).
func resolveBindHost(localMode bool, host string) string {
	host = strings.TrimSpace(host)
	if localMode && host == "" {
		return "127.0.0.1"
	}
	return host
}

func listenAddress(host string, port int) string {
	if strings.TrimSpace(host) == "" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port))
}

func displayHost(host string) string {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" || trimmed == "0.0.0.0" || trimmed == "::" {
		return "localhost"
	}
	return trimmed
}

func serverURL(host string, port int, basePath string) string {
	hostPort := net.JoinHostPort(displayHost(host), strconv.Itoa(port))
	url := "http://" + hostPort
	if basePath != "" {
		url += basePath
	}
	return url
}

// RunWithSignalHandling starts the server and handles OS signals for graceful shutdown.
func RunWithSignalHandling(cfg Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Run server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, cfg)
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
