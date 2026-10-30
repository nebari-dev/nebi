package main

import (
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Disconnect from the nebi server",
	Long: `Removes stored server URL and credentials.

Examples:
  nebi logout`,
	Args: cobra.NoArgs,
	RunE: runLogout,
}

func runLogout(cmd *cobra.Command, args []string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	serverURL, _ := s.LoadServerURL()
	if serverURL == "" {
		fmt.Fprintln(os.Stderr, "Not logged in.")
		return nil
	}

	if err := s.ClearCredentials(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Logged out from %s\n", serverURL)
	return nil
}
