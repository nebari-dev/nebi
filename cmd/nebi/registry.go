package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
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
	registryListJSON     bool
	registryLocal        bool
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
	registryListCmd.Flags().BoolVar(&registryListJSON, "json", false, "Output as JSON")
	registryListCmd.Flags().BoolVar(&registryLocal, "local", false, "Operate on local registry store instead of server")
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryRemoveCmd)

	registryAddCmd.Flags().StringVar(&registryAddName, "name", "", "Registry name (required)")
	registryAddCmd.Flags().StringVar(&registryAddURL, "url", "", "Registry URL (required)")
	registryAddCmd.Flags().StringVar(&registryAddUsername, "username", "", "Username for authentication")
	registryAddCmd.Flags().StringVar(&registryAddNamespace, "namespace", "", "Organization or namespace on the registry")
	registryAddCmd.Flags().BoolVar(&registryAddPwdStdin, "password-stdin", false, "Read password from stdin")
	registryAddCmd.Flags().BoolVar(&registryAddDefault, "default", false, "Set as default registry")
	registryAddCmd.Flags().BoolVar(&registryLocal, "local", false, "Operate on local registry store instead of server")

	registryAddCmd.MarkFlagRequired("name")
	registryAddCmd.MarkFlagRequired("url")
	registryAddCmd.MarkFlagRequired("namespace")

	registryRemoveCmd.Flags().BoolVarP(&registryRemoveForce, "force", "f", false, "Skip confirmation prompt")
	registryRemoveCmd.Flags().BoolVar(&registryLocal, "local", false, "Operate on local registry store instead of server")
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	if isLocalMode(cmd) {
		return runRegistryListLocal()
	}
	return runRegistryListServer()
}

func runRegistryListLocal() error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	registries, err := s.ListRegistries()
	if err != nil {
		return err
	}

	if len(registries) == 0 {
		if registryListJSON {
			return writeJSON([]store.LocalRegistry{})
		}
		fmt.Fprintln(os.Stderr, "No local registries configured.")
		return nil
	}

	if registryListJSON {
		return writeJSON(registries)
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

func runRegistryListServer() error {
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
		if registryListJSON {
			return writeJSON([]cliclient.Registry{})
		}
		fmt.Fprintln(os.Stderr, "No registries configured on server.")
		return nil
	}

	if registryListJSON {
		return writeJSON(registries)
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

	// Handle password input (same for both modes)
	if registryAddUsername != "" {
		if registryAddPwdStdin {
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				password = scanner.Text()
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("reading password from stdin: %w", err)
			}
		} else if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprint(os.Stderr, "Password: ")
			passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			password = string(passBytes)
		}
	}

	if isLocalMode(cmd) {
		return runRegistryAddLocal(password)
	}
	return runRegistryAddServer(password)
}

func runRegistryAddLocal(password string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	reg := &store.LocalRegistry{
		Name:      registryAddName,
		URL:       registryAddURL,
		Username:  registryAddUsername,
		IsDefault: registryAddDefault,
		Namespace: registryAddNamespace,
	}
	if err := s.CreateRegistry(reg); err != nil {
		return fmt.Errorf("creating local registry: %w", err)
	}

	if password != "" {
		cs := store.NewCredentialStore(s.DataDir())
		if err := cs.SetPassword(registryAddName, password); err != nil {
			return fmt.Errorf("storing credentials: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Added local registry '%s' (%s)\n", reg.Name, reg.URL)
	return nil
}

func runRegistryAddServer(password string) error {
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

	if isLocalMode(cmd) {
		return runRegistryRemoveLocal(name)
	}
	return runRegistryRemoveServer(name)
}

func runRegistryRemoveLocal(name string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	reg, err := s.GetRegistryByName(name)
	if err != nil {
		return err
	}

	if !registryRemoveForce {
		fmt.Fprintf(os.Stderr, "Remove local registry '%s'? [y/N] ", name)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	if err := s.DeleteRegistry(reg.ID); err != nil {
		return fmt.Errorf("deleting registry: %w", err)
	}

	cs := store.NewCredentialStore(s.DataDir())
	cs.DeletePassword(name) // Ignore error — credential may not exist

	fmt.Fprintf(os.Stderr, "Removed local registry '%s'\n", name)
	return nil
}

func runRegistryRemoveServer(name string) error {
	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	registryID, err := findRegistryByName(client, ctx, name)
	if err != nil {
		return err
	}

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
