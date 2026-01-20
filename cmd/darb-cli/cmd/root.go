package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nebi",
	Short: "Nebi CLI - Environment management with OCI registry support",
	Long: `Nebi is a CLI for managing Pixi environments and pushing/pulling them to OCI registries.

Examples:
  # Login to server
  nebi login https://nebi.company.com

  # Add and configure registries
  nebi registry add ds-team ghcr.io/myorg/data-science
  nebi registry set-default ds-team

  # Push/pull workspaces
  nebi push myworkspace:v1.0.0
  nebi pull myworkspace

  # Manage workspaces
  nebi workspace list
  nebi workspace info myworkspace
  nebi shell myworkspace`,
}

func Execute() error {
	return rootCmd.Execute()
}
