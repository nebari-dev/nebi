package main

import (
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/server"
	"github.com/spf13/cobra"
)

var (
	servePort int
	serveMode string
)

// @title Nebi API
// @version 1.0
// @description Multi-User Environment Management System API
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

Environment variables:
  NEBI_SERVER_PORT         Server port (default: 8460)
  NEBI_DATABASE_DRIVER     Database driver: sqlite, postgres
  NEBI_DATABASE_DSN        Database connection string
  NEBI_QUEUE_TYPE          Queue type: memory, valkey
  NEBI_AUTH_JWT_SECRET     JWT signing secret
  ADMIN_USERNAME           Bootstrap admin username
  ADMIN_PASSWORD           Bootstrap admin password`,
	Run: runServe,
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port to run server on (overrides config)")
	serveCmd.Flags().StringVarP(&serveMode, "mode", "m", "both", "Run mode: server, worker, or both")
}

func runServe(cmd *cobra.Command, args []string) {
	cfg := server.Config{
		Port:    servePort,
		Mode:    serveMode,
		Version: Version,
	}

	if err := server.RunWithSignalHandling(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
