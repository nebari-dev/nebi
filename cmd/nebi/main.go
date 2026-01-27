package main

import (
	"github.com/spf13/cobra"

	_ "github.com/aktech/darb/docs" // Load swagger docs
)

// Version is set via ldflags at build time
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "nebi",
	Short: "Nebi - Environment management with OCI registry support",
	Long: `Nebi is a CLI and server for managing Pixi environments and pushing/pulling them to OCI registries.

Examples:
  # Login and push a repo
  nebi login https://nebi.company.com
  nebi registry add ds-team ghcr.io/myorg/data-science --default
  nebi push myrepo:v1.0.0

  # Start the server
  nebi serve --port 8460`,
}

func init() {
	rootCmd.AddGroup(
		&cobra.Group{ID: "client", Title: "Client Commands:"},
		&cobra.Group{ID: "server", Title: "Server Commands:"},
	)

	loginCmd.GroupID = "client"
	logoutCmd.GroupID = "client"
	registryCmd.GroupID = "client"
	repoCmd.GroupID = "client"
	pushCmd.GroupID = "client"
	pullCmd.GroupID = "client"
	shellCmd.GroupID = "client"
	statusCmd.GroupID = "client"
	diffCmd.GroupID = "client"
	repairCmd.GroupID = "client"

	serveCmd.GroupID = "server"

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(repoCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(repairCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		osExit(1)
	}
}
