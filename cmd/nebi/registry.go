package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
)

var (
	registryAddUsername string
	registryAddPassword string
	registryAddDefault  bool
)

var registryCmd = &cobra.Command{
	Use:     "registry",
	Aliases: []string{"reg"},
	Short:   "Manage OCI registries",
	Long:    `Add, remove, list, and configure OCI registries for pushing and pulling environments.`,
}

var registryAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Add a named registry",
	Long: `Add a named OCI registry for storing workspaces.

Examples:
  # Add Docker Hub with credentials
  nebi registry add my-dhub docker.io -u myuser -p <token>

  # Add GitHub Container Registry
  nebi registry add ghcr ghcr.io/myorg -u myuser -p <token>

  # Add and set as default
  nebi registry add ds-team ghcr.io/myorg/data-science --default`,
	Args: cobra.ExactArgs(2),
	Run:  runRegistryAdd,
}

var registryListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all registries",
	Long:    `List all configured OCI registries.`,
	Args:    cobra.NoArgs,
	Run:     runRegistryList,
}

var registryRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a registry",
	Long: `Remove a named registry from the configuration.

Example:
  nebi registry remove ds-team`,
	Args: cobra.ExactArgs(1),
	Run:  runRegistryRemove,
}

var registrySetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default registry",
	Long: `Set a registry as the default, making the -r flag optional in most commands.

Example:
  nebi registry set-default ds-team`,
	Args: cobra.ExactArgs(1),
	Run:  runRegistrySetDefault,
}

func init() {
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryRemoveCmd)
	registryCmd.AddCommand(registrySetDefaultCmd)

	registryAddCmd.Flags().StringVarP(&registryAddUsername, "username", "u", "", "Registry username")
	registryAddCmd.Flags().StringVarP(&registryAddPassword, "password", "p", "", "Registry password or token")
	registryAddCmd.Flags().BoolVar(&registryAddDefault, "default", false, "Set as default registry")
}

func runRegistryAdd(cmd *cobra.Command, args []string) {
	name := args[0]
	url := args[1]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	req := cliclient.CreateRegistryRequest{
		Name:      name,
		URL:       url,
		Username:  &registryAddUsername,
		Password:  &registryAddPassword,
		IsDefault: &registryAddDefault,
	}

	resp, err := client.CreateRegistry(ctx, req)
	if err != nil {
		if cliclient.IsForbidden(err) {
			fmt.Fprintln(os.Stderr, "Error: Admin access required to add registries")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to add registry: %v\n", err)
		}
		os.Exit(1)
	}

	defaultMsg := ""
	if resp.IsDefault {
		defaultMsg = " (default)"
	}
	fmt.Printf("Added registry %q (%s)%s\n", resp.Name, resp.URL, defaultMsg)
}

func runRegistryList(cmd *cobra.Command, args []string) {
	client := mustGetClient()
	ctx := mustGetAuthContext()

	registries, err := client.ListRegistries(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list registries: %v\n", err)
		os.Exit(1)
	}

	if len(registries) == 0 {
		fmt.Println("No registries configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tDEFAULT")
	for _, reg := range registries {
		defaultMark := ""
		if reg.IsDefault {
			defaultMark = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", reg.Name, reg.URL, defaultMark)
	}
	w.Flush()
}

func runRegistryRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find registry by name
	reg, err := findRegistryByName(client, ctx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Delete by ID
	err = client.DeleteRegistry(ctx, reg.ID)
	if err != nil {
		if cliclient.IsForbidden(err) {
			fmt.Fprintln(os.Stderr, "Error: Admin access required to remove registries")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to remove registry: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("Removed registry %q\n", name)
}

func runRegistrySetDefault(cmd *cobra.Command, args []string) {
	name := args[0]

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Find registry by name
	reg, err := findRegistryByName(client, ctx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Update to set as default
	isDefault := true
	updateReq := cliclient.UpdateRegistryRequest{
		IsDefault: &isDefault,
	}

	_, err = client.UpdateRegistry(ctx, reg.ID, updateReq)
	if err != nil {
		if cliclient.IsForbidden(err) {
			fmt.Fprintln(os.Stderr, "Error: Admin access required to set default registry")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to set default registry: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("Set %q as default registry\n", name)
}

// findRegistryByName looks up a registry by name and returns it.
func findRegistryByName(client *cliclient.Client, ctx context.Context, name string) (*cliclient.Registry, error) {
	registries, err := client.ListRegistriesAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list registries: %v", err)
	}

	for _, reg := range registries {
		if reg.Name == name {
			return &reg, nil
		}
	}

	return nil, fmt.Errorf("registry %q not found", name)
}

// findDefaultRegistry finds the default registry.
func findDefaultRegistry(client *cliclient.Client, ctx context.Context) (*cliclient.Registry, error) {
	registries, err := client.ListRegistries(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list registries: %v", err)
	}

	for _, reg := range registries {
		if reg.IsDefault {
			return &reg, nil
		}
	}

	return nil, fmt.Errorf("no default registry set")
}
