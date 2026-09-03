package main

import (
	"context"
	"os"
	"strings"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for nebi.

To load completions:

Bash:
  $ source <(nebi completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ nebi completion bash > /etc/bash_completion.d/nebi
  # macOS:
  $ nebi completion bash > $(brew --prefix)/etc/bash_completion.d/nebi

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ nebi completion zsh > "${fpath[1]}/_nebi"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ nebi completion fish | source
  # To load completions for each session, execute once:
  $ nebi completion fish > ~/.config/fish/completions/nebi.fish

PowerShell:
  PS> nebi completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> nebi completion powershell > nebi.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

// completeWorkspaceNames returns completion for tracked workspace names.
func completeWorkspaceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	s, err := store.New()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer s.Close()

	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var names []string
	for _, ws := range workspaces {
		if strings.HasPrefix(ws.Name, toComplete) {
			names = append(names, ws.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeWorkspaceNamesOrPaths returns completion for workspace names with file fallback.
// This allows both workspace names and directory paths.
func completeWorkspaceNamesOrPaths(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}

	// If it looks like a path, let shell handle it
	if strings.HasPrefix(toComplete, "/") || strings.HasPrefix(toComplete, "./") || strings.HasPrefix(toComplete, "../") {
		return nil, cobra.ShellCompDirectiveDefault
	}

	s, err := store.New()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	defer s.Close()

	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}

	var names []string
	for _, ws := range workspaces {
		if strings.HasPrefix(ws.Name, toComplete) {
			names = append(names, ws.Name)
		}
	}

	// Allow file completion as fallback
	return names, cobra.ShellCompDirectiveDefault
}

// completeServerWorkspaceNames returns completion for server workspace names.
// This makes a network call to the configured server.
func completeServerWorkspaceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		// Not logged in or no server configured - fail silently
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ctx := context.Background()
	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, ws := range workspaces {
		if strings.HasPrefix(ws.Name, toComplete) {
			names = append(names, ws.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeWorkspaceRemove returns completion based on --remote flag.
// Uses server workspaces if --remote is set, otherwise local workspaces.
func completeWorkspaceRemove(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Check if --remote flag is set
	remote, _ := cmd.Flags().GetBool("remote")
	if remote {
		return completeServerWorkspaceNames(cmd, args, toComplete)
	}
	return completeWorkspaceNamesOrPaths(cmd, args, toComplete)
}

// completeServerWorkspaceRef returns completion for server workspace:tag refs.
// Completes workspace names from the server with NoSpace for tag addition.
func completeServerWorkspaceRef(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// If completing a tag (after colon), could fetch tags but skip for now
	if strings.Contains(toComplete, ":") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ctx := context.Background()
	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, ws := range workspaces {
		if strings.HasPrefix(ws.Name, toComplete) {
			names = append(names, ws.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoSpace
}
