package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage OCI registries on a server",
}

var regListServer string

var registryListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List registries on a server",
	Long: `List OCI registries configured on a nebi server.

Examples:
  nebi registry list -s work
  nebi registry list`,
	Args: cobra.NoArgs,
	RunE: runRegistryList,
}

func init() {
	registryListCmd.Flags().StringVarP(&regListServer, "server", "s", "", "Server name or URL (uses default if not set)")
	registryCmd.AddCommand(registryListCmd)
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	server, err := resolveServerFlag(regListServer)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient(server)
	if err != nil {
		return err
	}

	ctx := context.Background()

	registries, err := client.ListRegistries(ctx)
	if err != nil {
		return fmt.Errorf("listing registries: %w", err)
	}

	if len(registries) == 0 {
		fmt.Fprintln(os.Stderr, "No registries configured on server.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tDEFAULT")
	for _, r := range registries {
		def := ""
		if r.IsDefault {
			def = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.URL, def)
	}
	return w.Flush()
}
