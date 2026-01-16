package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pushTags []string
var pushRegistry string

var pushCmd = &cobra.Command{
	Use:   "push <env>",
	Short: "Push environment to registry",
	Long: `Push a local environment to an OCI registry with one or more tags.

Examples:
  # Push with single tag
  darb push myenv -t v1.0.0 -r ds-team

  # Push with multiple tags
  darb push myenv -t v1.0.0 -t latest -t stable -r ds-team

  # Push using default registry
  darb push myenv -t v1.0.0`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
	pushCmd.Flags().StringArrayVarP(&pushTags, "tag", "t", nil, "Tag(s) for the environment (repeatable)")
	pushCmd.Flags().StringVarP(&pushRegistry, "registry", "r", "", "Named registry (optional if default set)")
	pushCmd.MarkFlagRequired("tag")
}
