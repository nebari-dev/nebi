package main

import (
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

// Research prototype: rename local metadata without changing Pixi files or paths.
var workspaceRenameCmd = &cobra.Command{
	Use:   "rename <name|path|id::uuid> <new-name>",
	Short: "Rename a locally tracked workspace in Nebi",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateWorkspaceName(args[1]); err != nil {
			return err
		}
		s, err := store.New()
		if err != nil {
			return err
		}
		defer s.Close()
		ws, err := resolveLocalWorkspace(s, args[0])
		if err != nil {
			return err
		}
		if err := s.RenameWorkspace(ws.ID, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Renamed workspace %s to %q\n", ws.ID, args[1])
		return nil
	},
}

func init() {
	workspaceCmd.AddCommand(workspaceRenameCmd)
}
