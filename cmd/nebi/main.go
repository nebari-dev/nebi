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
	Long:  `Nebi manages Pixi workspaces locally and syncs them to remote servers.`,
	Example: `  # Track a workspace and push it to a server
  nebi init
  nebi server add work https://nebi.company.com
  nebi login work
  nebi push myworkspace:v1.0

  # Compare specs between directories or server versions
  nebi diff ./project-a ./project-b
  nebi diff myworkspace:v1 myworkspace:v2 -s work`,
}

func init() {
	rootCmd.AddGroup(
		&cobra.Group{ID: "workspace", Title: "Workspace Commands:"},
		&cobra.Group{ID: "sync", Title: "Sync Commands:"},
		&cobra.Group{ID: "server", Title: "Server Commands:"},
		&cobra.Group{ID: "admin", Title: "Admin Commands:"},
	)

	initCmd.GroupID = "workspace"
	workspaceCmd.GroupID = "workspace"
	shellCmd.GroupID = "workspace"
	runCmd.GroupID = "workspace"
	statusCmd.GroupID = "workspace"

	pushCmd.GroupID = "sync"
	pullCmd.GroupID = "sync"
	diffCmd.GroupID = "sync"
	publishCmd.GroupID = "sync"

	loginCmd.GroupID = "server"
	serverCmd.GroupID = "server"
	registryCmd.GroupID = "server"

	serveCmd.GroupID = "admin"

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
