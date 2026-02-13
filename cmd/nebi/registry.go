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
	registryCreateName     string
	registryCreateURL      string
	registryCreateUsername string
	registryCreateDefault  bool
	registryCreatePwdStdin bool
	registryDeleteForce    bool
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

var registryCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new OCI registry",
	Long: `Create a new OCI registry configuration on the server.

Examples:
  # Interactive - prompts for password
  nebi registry create --name ghcr --url ghcr.io --username myuser

  # Programmatic - read password from stdin
  echo "$TOKEN" | nebi registry create --name ghcr --url ghcr.io --username myuser --password-stdin

  # Public registry (no auth)
  nebi registry create --name dockerhub --url docker.io --default`,
	Args: cobra.NoArgs,
	RunE: runRegistryCreate,
}

var registryDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an OCI registry",
	Long: `Delete an OCI registry configuration from the server.

Examples:
  # Interactive - prompts for confirmation
  nebi registry delete ghcr

  # Skip confirmation
  nebi registry delete ghcr --force`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryDelete,
}

func init() {
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryCreateCmd)
	registryCmd.AddCommand(registryDeleteCmd)

	registryCreateCmd.Flags().StringVar(&registryCreateName, "name", "", "Registry name (required)")
	registryCreateCmd.Flags().StringVar(&registryCreateURL, "url", "", "Registry URL (required)")
	registryCreateCmd.Flags().StringVar(&registryCreateUsername, "username", "", "Username for authentication")
	registryCreateCmd.Flags().BoolVar(&registryCreatePwdStdin, "password-stdin", false, "Read password from stdin")
	registryCreateCmd.Flags().BoolVar(&registryCreateDefault, "default", false, "Set as default registry")

	registryCreateCmd.MarkFlagRequired("name")
	registryCreateCmd.MarkFlagRequired("url")

	registryDeleteCmd.Flags().BoolVarP(&registryDeleteForce, "force", "f", false, "Skip confirmation prompt")
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

func runRegistryCreate(cmd *cobra.Command, args []string) error {
	var password string

	// Handle password input
	if registryCreateUsername != "" {
		if registryCreatePwdStdin {
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
		Name: registryCreateName,
		URL:  registryCreateURL,
	}
	if registryCreateUsername != "" {
		req.Username = &registryCreateUsername
	}
	if password != "" {
		req.Password = &password
	}
	if registryCreateDefault {
		req.IsDefault = &registryCreateDefault
	}

	ctx := context.Background()
	registry, err := client.CreateRegistry(ctx, req)
	if err != nil {
		return fmt.Errorf("creating registry: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created registry '%s' (%s)\n", registry.Name, registry.URL)
	return nil
}

func runRegistryDelete(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("not implemented")
}
