package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version is set via ldflags at build time
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version of the Darb CLI and server (if connected).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Darb CLI:\n")
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Go version: %s\n", runtime.Version())
		fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)

		// Try to get server version
		apiClient := getAPIClient()
		ctx := getAuthContext()

		serverVersion, _, err := apiClient.SystemAPI.VersionGet(ctx).Execute()
		if err == nil {
			fmt.Printf("\nDarb Server:\n")
			for key, value := range serverVersion {
				fmt.Printf("  %s: %v\n", key, value)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
