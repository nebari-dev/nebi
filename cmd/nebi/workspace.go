package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage tracked workspaces",
}

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List tracked workspaces with status",
	Args:    cobra.NoArgs,
	RunE:    runWorkspaceList,
}

func init() {
	workspaceCmd.AddCommand(workspaceListCmd)
}

func runWorkspaceList(cmd *cobra.Command, args []string) error {
	store, err := localstore.NewStore()
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return err
	}

	if len(idx.Workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "No tracked workspaces. Run 'nebi init' in a pixi workspace to get started.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATH\tSTATUS")
	for _, ws := range idx.Workspaces {
		status := store.ComputeStatus(ws)
		fmt.Fprintf(w, "%s\t%s\t%s\n", ws.Name, ws.Path, status)
	}
	return w.Flush()
}
