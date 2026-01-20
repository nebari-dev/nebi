package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openteams-ai/darb/cli/client"
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
	Long: `Add a named OCI registry for storing environments.

Examples:
  # Add Docker Hub with credentials
  darb registry add my-dhub docker.io -u myuser -p <token>

  # Add GitHub Container Registry
  darb registry add ghcr ghcr.io/myorg -u myuser -p <token>

  # Add and set as default
  darb registry add ds-team ghcr.io/myorg/data-science --default`,
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
  darb registry remove ds-team`,
	Args: cobra.ExactArgs(1),
	Run:  runRegistryRemove,
}

var registrySetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default registry",
	Long: `Set a registry as the default, making the -r flag optional in most commands.

Example:
  darb registry set-default ds-team`,
	Args: cobra.ExactArgs(1),
	Run:  runRegistrySetDefault,
}

func init() {
	rootCmd.AddCommand(registryCmd)
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

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	req := client.HandlersCreateRegistryRequest{
		Name:      name,
		Url:       url,
		Username:  &registryAddUsername,
		Password:  &registryAddPassword,
		IsDefault: &registryAddDefault,
	}

	resp, httpResp, err := apiClient.AdminAPI.AdminRegistriesPost(ctx).Registry(req).Execute()
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 403 {
			fmt.Fprintln(os.Stderr, "Error: Admin access required to add registries")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to add registry: %v\n", err)
		}
		os.Exit(1)
	}

	defaultMsg := ""
	if resp.GetIsDefault() {
		defaultMsg = " (default)"
	}
	fmt.Printf("Added registry %q (%s)%s\n", resp.GetName(), resp.GetUrl(), defaultMsg)
}

func runRegistryList(cmd *cobra.Command, args []string) {
	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Try admin endpoint first for full details, fall back to public
	registries, _, err := apiClient.AdminAPI.AdminRegistriesGet(ctx).Execute()
	if err != nil {
		// Try public endpoint
		registries, _, err = apiClient.RegistriesAPI.RegistriesGet(ctx).Execute()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to list registries: %v\n", err)
			os.Exit(1)
		}
	}

	if len(registries) == 0 {
		fmt.Println("No registries configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tDEFAULT")
	for _, reg := range registries {
		defaultMark := ""
		if reg.GetIsDefault() {
			defaultMark = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", reg.GetName(), reg.GetUrl(), defaultMark)
	}
	w.Flush()
}

func runRegistryRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find registry by name
	reg, err := findRegistryByName(apiClient, ctx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Delete by ID
	httpResp, err := apiClient.AdminAPI.AdminRegistriesIdDelete(ctx, reg.GetId()).Execute()
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 403 {
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

	apiClient := mustGetClient()
	ctx := mustGetAuthContext()

	// Find registry by name
	reg, err := findRegistryByName(apiClient, ctx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Update to set as default
	isDefault := true
	updateReq := client.HandlersUpdateRegistryRequest{
		IsDefault: &isDefault,
	}

	_, httpResp, err := apiClient.AdminAPI.AdminRegistriesIdPut(ctx, reg.GetId()).Registry(updateReq).Execute()
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 403 {
			fmt.Fprintln(os.Stderr, "Error: Admin access required to set default registry")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to set default registry: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("Set %q as default registry\n", name)
}

// findRegistryByName looks up a registry by name and returns it
func findRegistryByName(apiClient *client.APIClient, ctx context.Context, name string) (*client.HandlersRegistryResponse, error) {
	registries, _, err := apiClient.AdminAPI.AdminRegistriesGet(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to list registries: %v", err)
	}

	for _, reg := range registries {
		if reg.GetName() == name {
			return &reg, nil
		}
	}

	return nil, fmt.Errorf("registry %q not found", name)
}
