package main

import (
	"os"

	"github.com/spf13/cobra"

	_ "github.com/aktech/darb/docs" // Load swagger docs
)

// Version is set via ldflags at build time
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "nebi",
	Short: "Nebi - Environment management with OCI registry support",
	Long: `Nebi is a CLI and server for managing Pixi environments and pushing/pulling them to OCI registries.

Server commands:
  nebi serve              Start the Nebi server

Client commands:
  nebi login <url>        Login to a Nebi server
  nebi logout             Logout from the server

  nebi registry add       Add an OCI registry
  nebi registry list      List registries
  nebi registry remove    Remove a registry
  nebi registry set-default  Set default registry

  nebi workspace list     List workspaces
  nebi workspace info     Show workspace details
  nebi workspace delete   Delete a workspace
  nebi workspace tags     List published tags

  nebi push <ws>:<tag>    Push workspace to registry
  nebi pull <ws>[:<tag>]  Pull workspace from server
  nebi shell <ws>         Activate workspace shell

Examples:
  # Start the server
  nebi serve --port 8460

  # Login and push a workspace
  nebi login https://nebi.company.com
  nebi registry add ds-team ghcr.io/myorg/data-science --default
  nebi push myworkspace:v1.0.0`,
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(diffCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
