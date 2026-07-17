package main

import (
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/server"
	"github.com/spf13/cobra"
)

var (
	serveHost string
	servePort int
	serveMode string
)

// @title Nebi API
// @version 1.0
// @description Multi-User Workspace Management System API
// @host localhost:8460
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run a Nebi server instance (for admins/operators)",
	Long: `Start the Nebi server with API and/or worker components.

Examples:
  nebi serve                    # Run both API server and worker
  nebi serve --mode server      # Run API server only
  nebi serve --mode worker      # Run worker only
  nebi serve --port 8080        # Override port
  nebi serve --host 127.0.0.1   # Bind only to loopback

Environment variables:
  NEBI_MODE                         Server mode: "local" or "team" (default: "team")
  NEBI_SERVER_HOST                  Bind host/IP (e.g. 127.0.0.1). If unset: team mode binds all
                                    interfaces; local mode binds loopback only
  NEBI_SERVER_PORT                  Server port (default: 8460)
  NEBI_SERVER_MODE                  Server environment: "development" or "production" (default: "development")
  NEBI_SERVER_BASE_PATH             URL path prefix for reverse proxy (e.g. "/nebi")
  NEBI_DATABASE_DRIVER              Database driver: "sqlite" or "postgres" (default: "sqlite")
  NEBI_DATABASE_DSN                 Database connection string (default: "./nebi.db")
  NEBI_DATABASE_MAX_IDLE_CONNS      Max idle connections in pool — Postgres only (default: 10)
  NEBI_DATABASE_MAX_OPEN_CONNS      Max open connections in pool — Postgres only (default: 100)
  NEBI_DATABASE_CONN_MAX_LIFETIME   Connection max lifetime in minutes — Postgres only (default: 60)
  NEBI_AUTH_TYPE                    Authentication type: "basic" or "oidc" (default: "basic")
  NEBI_AUTH_JWT_SECRET              JWT signing secret (default: "change-me-in-production")
  NEBI_AUTH_OIDC_ISSUER_URL         OIDC provider issuer URL
  NEBI_AUTH_OIDC_CLIENT_ID          OIDC client ID
  NEBI_AUTH_OIDC_CLIENT_SECRET      OIDC client secret
  NEBI_AUTH_OIDC_REDIRECT_URL       OIDC redirect URL (default: http://localhost:8460/api/v1/auth/oidc/callback)
  NEBI_QUEUE_TYPE                   Job queue type: "memory" or "valkey" (default: "memory")
  NEBI_QUEUE_VALKEY_ADDR            Valkey server address (default: "localhost:6379")
  NEBI_LOG_FORMAT                   Log format: "text" or "json" (default: "text")
  NEBI_LOG_LEVEL                    Log level: "debug", "info", "warn", "error" (default: "info")
  NEBI_PACKAGE_MANAGER_DEFAULT_TYPE Default package manager: "pixi" or "uv" (default: "pixi")
  NEBI_PACKAGE_MANAGER_PIXI_PATH    Custom pixi binary path (optional)
  NEBI_PACKAGE_MANAGER_UV_PATH      Custom uv binary path (optional)
  NEBI_STORAGE_WORKSPACES_DIR       Directory for workspace storage (default: "./data/workspaces")
  NEBI_AUTH_TOKEN                   Auth token for auto-connect when in local mode
  NEBI_REMOTE_URL                   Remote server URL for auto-connect when in local mode
  ADMIN_USERNAME                    Bootstrap admin username
  ADMIN_PASSWORD                    Bootstrap admin password
  ADMIN_EMAIL                       Bootstrap admin email (default: "<username>@nebi.local")`,
	Run: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveHost, "host", "", "Bind host/IP (overrides config), e.g. 127.0.0.1. Empty: all interfaces in team mode, loopback in local mode")
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port to run server on (overrides config)")
	serveCmd.Flags().StringVarP(&serveMode, "mode", "m", "both", "Run mode: server, worker, or both")
}

func runServe(cmd *cobra.Command, args []string) {
	cfg := server.Config{
		Host:    serveHost,
		Port:    servePort,
		Mode:    serveMode,
		Version: Version,
		Commit:  Commit,
	}

	if err := server.RunWithSignalHandling(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
