package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var envListRegistry string
var envInfoRegistry string

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
	Long:  `Create, delete, list, and inspect local and remote environments.`,
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	Long: `List local environments or remote environments from a registry.

Examples:
  # List local envs
  darb env list

  # List remote envs from registry
  darb env list -r ds-team`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var envListTagsCmd = &cobra.Command{
	Use:   "tags <env>",
	Short: "List tags for an environment",
	Long: `List all tags for an environment in a registry.

Example:
  darb env list tags data-science -r ds-team`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var envCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a local environment",
	Long: `Create a new local Pixi environment.

Example:
  darb env create myenv`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var envDeleteCmd = &cobra.Command{
	Use:   "delete <env>",
	Short: "Delete a local environment",
	Long: `Delete a local environment.

Example:
  darb env delete myenv`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var envInfoCmd = &cobra.Command{
	Use:   "info <env>",
	Short: "Show environment details",
	Long: `Show detailed information about a local or remote environment.

Examples:
  # Local env info
  darb env info myenv

  # Remote env info
  darb env info data-science -r ds-team`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(envCmd)

	// env list
	envCmd.AddCommand(envListCmd)
	envListCmd.Flags().StringVarP(&envListRegistry, "registry", "r", "", "Named registry (optional, lists remote envs)")

	// env list tags (subcommand of list)
	envListCmd.AddCommand(envListTagsCmd)
	envListTagsCmd.Flags().StringVarP(&envListRegistry, "registry", "r", "", "Named registry (optional if default set)")

	// env create
	envCmd.AddCommand(envCreateCmd)

	// env delete
	envCmd.AddCommand(envDeleteCmd)

	// env info
	envCmd.AddCommand(envInfoCmd)
	envInfoCmd.Flags().StringVarP(&envInfoRegistry, "registry", "r", "", "Named registry (optional, shows remote env)")
}
