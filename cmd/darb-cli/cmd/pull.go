package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pullTag string
var pullDigest string
var pullRegistry string

var pullCmd = &cobra.Command{
	Use:   "pull [env]",
	Short: "Pull environment from registry",
	Long: `Pull an environment from an OCI registry by tag or digest.

Examples:
  # Pull specific tag
  darb pull data-science -t v1.0.0 -r ds-team

  # Pull latest (default tag)
  darb pull data-science -r ds-team

  # Pull by digest (automation-friendly, immutable)
  darb pull -d sha256:abc123def -r ds-team`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().StringVarP(&pullTag, "tag", "t", "", "Tag to pull (default: latest)")
	pullCmd.Flags().StringVarP(&pullDigest, "digest", "d", "", "OCI digest (immutable reference)")
	pullCmd.Flags().StringVarP(&pullRegistry, "registry", "r", "", "Named registry (optional if default set)")
}
