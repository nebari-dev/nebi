package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/registry"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage OCI registries",
	Long: `Manage OCI registries. With --local, or when no server is logged in, the
commands work on the registries of this machine (the ones the desktop app
and nebi-web use; passwords are kept in the OS keyring).`,
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

var registrySetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Mark an existing OCI registry as the default",
	Long: `Mark an existing OCI registry as the default. Any previously
default registry is unset atomically.

Examples:
  nebi registry set-default quay
  nebi registry set-default quay --local`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistrySetDefault,
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
	registryCmd.AddCommand(registrySetDefaultCmd)

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

	registrySetDefaultCmd.Flags().BoolVar(&registryLocal, "local", false, "Operate on local registry store instead of server")
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	if isLocalMode(cmd) {
		return runRegistryListLocal(cmd)
	}
	return runRegistryListServer()
}

func runRegistryListLocal(cmd *cobra.Command) error {
	return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
		registries, err := st.SDK.Registry.ListRegistries(ctx)
		if err != nil {
			return err
		}
		if registryListJSON {
			return writeJSON(registries)
		}
		if len(registries) == 0 {
			fmt.Fprintln(os.Stderr, "No local registries configured.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tURL\tNAMESPACE\tDEFAULT")
		for _, r := range registries {
			def := ""
			if r.IsDefault {
				def = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.URL, r.Namespace, def)
		}
		return w.Flush()
	})
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
		return runRegistryAddLocal(cmd, password)
	}
	return runRegistryAddServer(password)
}

func runRegistryAddLocal(cmd *cobra.Command, password string) error {
	return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
		reg, err := st.SDK.Registry.AddRegistry(ctx, shared.AddRegistryRequest{
			Name:      registryAddName,
			URL:       registryAddURL,
			Namespace: registryAddNamespace,
			Username:  registryAddUsername,
			Password:  password,
			IsDefault: registryAddDefault,
		})
		if err != nil {
			return fmt.Errorf("creating local registry: %w", explainSDKError(err))
		}
		fmt.Fprintf(os.Stderr, "Added local registry '%s' (%s)\n", reg.Name, reg.URL)
		return nil
	})
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

func runRegistrySetDefault(cmd *cobra.Command, args []string) error {
	name := args[0]
	if isLocalMode(cmd) {
		return runRegistrySetDefaultLocal(cmd, name)
	}
	return runRegistrySetDefaultServer(name)
}

func runRegistrySetDefaultLocal(cmd *cobra.Command, name string) error {
	return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
		reg, err := resolveLocalRegistry(ctx, st, name)
		if err != nil {
			return err
		}
		yes := true
		updated, err := st.SDK.Registry.UpdateRegistry(ctx, reg.ID, registry.UpdateRegistryRequest{IsDefault: &yes})
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Default registry set to '%s' (%s)\n", updated.Name, updated.URL)
		return nil
	})
}

func runRegistrySetDefaultServer(name string) error {
	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	id, err := resolveRegistryID(client, ctx, name)
	if err != nil {
		return err
	}
	yes := true
	reg, err := client.UpdateRegistry(ctx, id, cliclient.UpdateRegistryRequest{
		IsDefault: &yes,
	})
	if err != nil {
		return fmt.Errorf("set-default failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Default registry set to '%s' (%s)\n", reg.Name, reg.URL)
	return nil
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	if isLocalMode(cmd) {
		return runRegistryRemoveLocal(cmd, name)
	}
	return runRegistryRemoveServer(name)
}

func runRegistryRemoveLocal(cmd *cobra.Command, name string) error {
	return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
		reg, err := resolveLocalRegistry(ctx, st, name)
		if err != nil {
			return err
		}
		if !registryRemoveForce {
			fmt.Fprintf(os.Stderr, "Remove local registry '%s'? [y/N] ", reg.Name)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Fprintln(os.Stderr, "Aborted.")
				return nil
			}
		}
		if err := st.SDK.Registry.RemoveRegistry(ctx, reg.ID); err != nil {
			return fmt.Errorf("deleting registry: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Removed registry '%s'\n", reg.Name)
		return nil
	})
}

// resolveLocalRegistry finds a registry of this machine by name or ID.
func resolveLocalRegistry(ctx context.Context, st *client.Stack, ref string) (*shared.RegistryItem, error) {
	reg, err := st.SDK.Registry.ResolveRegistry(ctx, ref)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("registry '%s' not found", ref)
	}
	return reg, err
}

// newRegistryUpdateCmd edits a registry of this machine.
func newRegistryUpdateCmd() *cobra.Command {
	var (
		newName, url, namespace, username string
		passwordStdin, clearPassword      bool
		makeDefault                       bool
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Edit a registry on this machine",
		Long: `Edit a registry on this machine. Only the flags you pass change.

Examples:
  nebi registry update quay --namespace nebari
  echo "$TOKEN" | nebi registry update ghcr --username me --password-stdin
  nebi registry update ghcr --name github`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLocalFlags(cmd)
			var req registry.UpdateRegistryRequest
			flags := cmd.Flags()
			if flags.Changed("name") {
				req.Name = &newName
			}
			if flags.Changed("url") {
				req.URL = &url
			}
			if flags.Changed("namespace") {
				req.Namespace = &namespace
			}
			if flags.Changed("username") {
				req.Username = &username
			}
			if makeDefault {
				req.IsDefault = &makeDefault
			}
			if clearPassword {
				empty := ""
				req.Password = &empty
			} else if passwordStdin {
				scanner := bufio.NewScanner(os.Stdin)
				pw := ""
				if scanner.Scan() {
					pw = scanner.Text()
				}
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("reading password from stdin: %w", err)
				}
				req.Password = &pw
			}
			return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
				reg, err := resolveLocalRegistry(ctx, st, args[0])
				if err != nil {
					return err
				}
				updated, err := st.SDK.Registry.UpdateRegistry(ctx, reg.ID, req)
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Updated registry '%s' (%s)\n", updated.Name, updated.URL)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&newName, "name", "", "New registry name")
	cmd.Flags().StringVar(&url, "url", "", "Registry URL")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Organization or namespace on the registry")
	cmd.Flags().StringVar(&username, "username", "", "Username for authentication")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the new password from stdin")
	cmd.Flags().BoolVar(&clearPassword, "clear-password", false, "Remove the stored password")
	cmd.Flags().BoolVar(&makeDefault, "default", false, "Make this the default registry")
	return cmd
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
