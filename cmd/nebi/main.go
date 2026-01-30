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

Local commands:
  nebi init               Register current directory as a tracked workspace
  nebi workspace list     List tracked workspaces with status
  nebi diff               Show changes since last commit
  nebi commit             Snapshot current spec files

Server commands:
  nebi login              Authenticate with a server
  nebi push               Push spec files to a server
  nebi pull               Pull spec files from a server
  nebi serve              Start the Nebi server`,
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(commitCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
