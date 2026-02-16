package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage OCI registries on the server",
}

var (
	registryAddName      string
	registryAddURL       string
	registryAddUsername  string
	registryAddNamespace string
	registryAddDefault   bool
	registryAddPwdStdin  bool
	registryRemoveForce  bool
)

var registryListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List registries on the server",
	Long: `List OCI registries configured on the nebi server.

Examples:
  nebi registry list`,
	Args: cobra.NoArgs,
	RunE: runRegistryList,
}

var registryAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an OCI registry",
	Long: `Add an OCI registry configuration on the server.

Examples:
  # Interactive - prompts for password
  nebi registry add --name ghcr --url ghcr.io --username myuser

  # Programmatic - read password from stdin
  echo "$TOKEN" | nebi registry add --name quay --url quay.io --namespace nebari_environments --username myuser --password-stdin

  # Public registry (no auth)
  nebi registry add --name dockerhub --url docker.io --default`,
	Args: cobra.NoArgs,
	RunE: runRegistryAdd,
}

var registryRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove an OCI registry",
	Long: `Remove an OCI registry configuration from the server.

Examples:
  # Interactive - prompts for confirmation
  nebi registry remove ghcr

  # Skip confirmation
  nebi registry remove ghcr --force`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryRemove,
}

func init() {
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryRemoveCmd)

	registryAddCmd.Flags().StringVar(&registryAddName, "name", "", "Registry name (required)")
	registryAddCmd.Flags().StringVar(&registryAddURL, "url", "", "Registry URL (required)")
	registryAddCmd.Flags().StringVar(&registryAddUsername, "username", "", "Username for authentication")
	registryAddCmd.Flags().StringVar(&registryAddNamespace, "namespace", "", "Organization or namespace on the registry")
	registryAddCmd.Flags().BoolVar(&registryAddPwdStdin, "password-stdin", false, "Read password from stdin")
	registryAddCmd.Flags().BoolVar(&registryAddDefault, "default", false, "Set as default registry")

	registryAddCmd.MarkFlagRequired("name")
	registryAddCmd.MarkFlagRequired("url")
	registryAddCmd.MarkFlagRequired("namespace")

	registryRemoveCmd.Flags().BoolVarP(&registryRemoveForce, "force", "f", false, "Skip confirmation prompt")
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	client, err := getAuthenticatedClient()
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

func runRegistryAdd(cmd *cobra.Command, args []string) error {
	var password string

	// Handle password input
	if registryAddUsername != "" {
		if registryAddPwdStdin {
			// Read password from stdin
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				password = scanner.Text()
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("reading password from stdin: %w", err)
			}
		} else if term.IsTerminal(int(os.Stdin.Fd())) {
			// Interactive prompt
			fmt.Fprint(os.Stderr, "Password: ")
			passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			password = string(passBytes)
		}
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	req := cliclient.CreateRegistryRequest{
		Name: registryAddName,
		URL:  registryAddURL,
	}
	if registryAddUsername != "" {
		req.Username = &registryAddUsername
	}
	if password != "" {
		req.Password = &password
	}
	if registryAddDefault {
		req.IsDefault = &registryAddDefault
	}
	if registryAddNamespace != "" {
		req.Namespace = &registryAddNamespace
	}

	ctx := context.Background()
	registry, err := client.CreateRegistry(ctx, req)
	if err != nil {
		return fmt.Errorf("creating registry: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Added registry '%s' (%s)\n", registry.Name, registry.URL)
	return nil
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Find the registry by name
	registryID, err := findRegistryByName(client, ctx, name)
	if err != nil {
		return err
	}

	// Confirm unless --force
	if !registryRemoveForce {
		fmt.Fprintf(os.Stderr, "Remove registry '%s'? [y/N] ", name)

		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	if err := client.DeleteRegistry(ctx, registryID); err != nil {
		return fmt.Errorf("deleting registry: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Removed registry '%s'\n", name)
	return nil
}

// findRegistryByName finds a registry by name and returns its ID.
func findRegistryByName(client *cliclient.Client, ctx context.Context, name string) (string, error) {
	registries, err := client.ListRegistries(ctx)
	if err != nil {
		return "", fmt.Errorf("listing registries: %w", err)
	}

	for _, r := range registries {
		if r.Name == name {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("registry '%s' not found", name)
}
