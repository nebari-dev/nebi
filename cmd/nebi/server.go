package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage nebi servers",
}

var serverAddCmd = &cobra.Command{
	Use:   "add <name> <server-url>",
	Short: "Register a server",
	Long: `Registers a nebi server globally so it can be referenced by name.

Example:
  nebi server add work https://nebi.company.com
  nebi push work myworkspace:v1.0`,
	Args: cobra.ExactArgs(2),
	RunE: runServerAdd,
}

var serverListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List registered servers",
	Args:    cobra.NoArgs,
	RunE:    runServerList,
}

var serverRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a registered server",
	Args:    cobra.ExactArgs(1),
	RunE:    runServerRemove,
}

func init() {
	serverCmd.AddCommand(serverAddCmd)
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverRemoveCmd)
}

func runServerAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	url := args[1]

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	if _, exists := idx.Servers[name]; exists {
		return fmt.Errorf("server '%s' already exists; remove it first", name)
	}

	isFirst := len(idx.Servers) == 0
	idx.Servers[name] = url

	if err := store.SaveIndex(idx); err != nil {
		return err
	}

	// Auto-set as default if this is the first server
	if isFirst {
		cfg, err := localstore.LoadConfig()
		if err != nil {
			return err
		}
		cfg.DefaultServer = name
		if err := localstore.SaveConfig(cfg); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Server '%s' added: %s (set as default)\n", name, url)
	} else {
		fmt.Fprintf(os.Stderr, "Server '%s' added: %s\n", name, url)
	}
	return nil
}

func runServerList(cmd *cobra.Command, args []string) error {
	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	if len(idx.Servers) == 0 {
		fmt.Fprintln(os.Stderr, "No servers configured. Run 'nebi server add <name> <url>' to add one.")
		return nil
	}

	cfg, _ := localstore.LoadConfig()
	defaultServer := ""
	if cfg != nil {
		defaultServer = cfg.DefaultServer
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tDEFAULT")
	for name, url := range idx.Servers {
		def := ""
		if name == defaultServer {
			def = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, url, def)
	}
	return w.Flush()
}

func runServerRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	if _, exists := idx.Servers[name]; !exists {
		return fmt.Errorf("server '%s' not found", name)
	}

	delete(idx.Servers, name)

	if err := store.SaveIndex(idx); err != nil {
		return err
	}

	// Clear default if we just removed it
	cfg, _ := localstore.LoadConfig()
	if cfg != nil && cfg.DefaultServer == name {
		cfg.DefaultServer = ""
		_ = localstore.SaveConfig(cfg)
	}

	fmt.Fprintf(os.Stderr, "Server '%s' removed\n", name)
	return nil
}

// currentWorkspace returns the store and workspace for the current directory.
func currentWorkspace() (*localstore.Store, *localstore.Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("getting working directory: %w", err)
	}

	store, err := localstore.NewStore()
	if err != nil {
		return nil, nil, err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return nil, nil, err
	}

	ws, ok := idx.Workspaces[cwd]
	if !ok {
		return nil, nil, fmt.Errorf("current directory is not a tracked workspace; run 'nebi init' first")
	}

	return store, ws, nil
}
