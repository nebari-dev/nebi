package main

import (
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/server"
	"github.com/spf13/cobra"

	_ "github.com/nebari-dev/nebi/internal/swagger" // Load swagger docs
)

// Version is set via ldflags at build time.
var Version = "dev"

// Commit is the git commit hash, set via ldflags at build time.
var Commit = ""

var (
	host          string
	port          int
	componentMode string
)

// @title Nebi API
// @version 1.0
// @description Multi-User Workspace Management System API
// @host localhost:8460
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
var rootCmd = &cobra.Command{
	Use:   "nebi-server",
	Short: "Run the Nebi team server",
	Long: `Start the Nebi team-mode HTTP API server and/or worker.

Examples:
  nebi-server                    # Run both API server and worker
  nebi-server --mode server      # Run API server only
  nebi-server --mode worker      # Run worker only
  nebi-server --port 8080        # Override port
  nebi-server --host 127.0.0.1   # Bind only to loopback`,
	Run: run,
}

func init() {
	rootCmd.Flags().StringVar(&host, "host", "", "Bind host/IP (overrides config), e.g. 127.0.0.1")
	rootCmd.Flags().IntVarP(&port, "port", "p", 0, "Port to run server on (overrides config)")
	rootCmd.Flags().StringVarP(&componentMode, "mode", "m", "both", "Run mode: server, worker, or both")
}

func run(cmd *cobra.Command, args []string) {
	cfg := server.Config{
		Host:        host,
		Port:        port,
		Mode:        componentMode,
		RuntimeMode: config.ModeTeam,
		Version:     Version,
		Commit:      Commit,
	}

	if err := server.RunWithSignalHandling(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
