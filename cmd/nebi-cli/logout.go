package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Disconnect from the nebi server",
	Long: `Removes the stored server URL and credentials. Projects you pulled stay
on this machine and keep working.

Examples:
  nebi logout`,
	Args: cobra.NoArgs,
	RunE: runLogout,
}

func runLogout(cmd *cobra.Command, args []string) error {
	stack, closeStack, err := client.OpenCLI(io.Discard)
	if err != nil {
		return err
	}
	defer closeStack()

	conn, err := stack.SDK.Remote.GetConnection(cmd.Context())
	if err != nil {
		return err
	}
	if !conn.Connected {
		fmt.Fprintln(os.Stderr, "Not logged in.")
		return nil
	}
	if err := stack.SDK.Remote.Disconnect(cmd.Context()); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Logged out from %s\n", conn.URL)
	return nil
}
