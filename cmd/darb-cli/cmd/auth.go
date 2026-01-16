package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login <url>",
	Short: "Login to Darb server",
	Long: `Login to a Darb server to enable server mode.

Example:
  darb login https://darb.company.com`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from server",
	Long:  `Logout from the Darb server and return to direct mode.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Error: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
}
