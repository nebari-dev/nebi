package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "darb",
	Short: "Darb CLI - Environment management with OCI registry support",
	Long: `Darb is a CLI for managing Pixi environments and pushing/pulling them to OCI registries.

Examples:
  # Add and configure registries
  darb registry add ds-team ghcr.io/myorg/data-science
  darb registry set-default ds-team

  # Push/pull environments
  darb push myenv -t v1.0.0
  darb pull data-science -t latest

  # Manage local environments
  darb env create myenv
  darb env list
  darb shell myenv

  # Server mode (optional)
  darb login https://darb.company.com`,
}

func Execute() error {
	return rootCmd.Execute()
}
