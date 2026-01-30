package main

import (
	"os"

	"github.com/spf13/cobra"

	_ "github.com/nebari-dev/nebi/docs" // Load swagger docs
)

// Version is set via ldflags at build time
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "nebi",
	Short: "Nebi - Local-first environment management for Pixi",
	Long: `Nebi manages Pixi workspaces locally and syncs them to remote servers.

Examples:
  # Track a workspace and push it to a server
  nebi init
  nebi server add work https://nebi.company.com
  nebi login work
  nebi push myworkspace:v1.0

  # Compare specs between directories or server versions
  nebi diff ./project-a ./project-b
  nebi diff myworkspace:v1 myworkspace:v2 -s work

  # Run a server instance (admins only)
  nebi serve --port 8460`,
}

func init() {
	rootCmd.AddGroup(
		&cobra.Group{ID: "workspace", Title: "Workspace Commands:"},
		&cobra.Group{ID: "connection", Title: "Connection Commands:"},
		&cobra.Group{ID: "admin", Title: "Admin Commands:"},
	)

	initCmd.GroupID = "workspace"
	workspaceCmd.GroupID = "workspace"
	diffCmd.GroupID = "workspace"
	pushCmd.GroupID = "workspace"
	pullCmd.GroupID = "workspace"
	shellCmd.GroupID = "workspace"
	activateCmd.GroupID = "workspace"

	loginCmd.GroupID = "connection"
	serverCmd.GroupID = "connection"

	serveCmd.GroupID = "admin"

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(activateCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
