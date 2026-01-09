package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var registriesCmd = &cobra.Command{
	Use:     "registries",
	Aliases: []string{"reg"},
	Short:   "List available OCI registries",
	Long:    `List OCI registries available for publishing environments.`,
}

func init() {
	rootCmd.AddCommand(registriesCmd)
}

// List registries
var registriesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available registries",
	Long:  `List all OCI registries available for publishing environments.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		registries, httpResp, err := apiClient.RegistriesAPI.RegistriesGet(ctx).Execute()
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == 401 {
				return fmt.Errorf("not logged in. Run 'darb login' first")
			}
			return fmt.Errorf("failed to list registries: %w", err)
		}

		if len(registries) == 0 {
			fmt.Println("No registries configured.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tURL\tDEFAULT")
		for _, reg := range registries {
			defaultStr := ""
			if reg.GetIsDefault() {
				defaultStr = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				reg.Id,
				reg.Name,
				reg.Url,
				defaultStr,
			)
		}
		w.Flush()

		return nil
	},
}

func init() {
	registriesCmd.AddCommand(registriesListCmd)
}
