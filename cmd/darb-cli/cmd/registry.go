package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage OCI registries",
	Long:  `Add, remove, list, and configure OCI registries for pushing and pulling environments.`,
}

var registryAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Add a named registry",
	Long: `Add a named OCI registry for storing environments.

Examples:
  darb registry add ds-team ghcr.io/myorg/data-science
  darb registry add infra-team ghcr.io/myorg/infra`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registries",
	Long:  `List all configured OCI registries.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var registryRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a registry",
	Long: `Remove a named registry from the configuration.

Example:
  darb registry remove ds-team`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var registrySetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default registry",
	Long: `Set a registry as the default, making the -r flag optional in most commands.

Example:
  darb registry set-default ds-team`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(registryCmd)
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryRemoveCmd)
	registryCmd.AddCommand(registrySetDefaultCmd)
}
