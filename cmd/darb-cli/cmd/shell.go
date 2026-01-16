package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shellRegistry string

var shellCmd = &cobra.Command{
	Use:   "shell <env>",
	Short: "Activate environment shell",
	Long: `Activate an environment shell using pixi shell.

Examples:
  # Local env
  darb shell myenv

  # Remote env (pulls if needed)
  darb shell data-science -r ds-team`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellRegistry, "registry", "r", "", "Named registry (optional if default set)")
}
