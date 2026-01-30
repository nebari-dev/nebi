package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login <server-url-or-remote-name>",
	Short: "Authenticate with a nebi server",
	Long: `Authenticates with a nebi server and stores the credential.
Accepts a URL or a configured server name.

Examples:
  nebi login https://nebi.company.com
  nebi login work
  nebi login work --token <api-token>`,
	Args: cobra.ExactArgs(1),
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "API token (skip interactive login)")
}

// resolveServerURL resolves a server name to its URL, or returns the argument as-is if it looks like a URL.
func resolveServerURL(arg string) (string, error) {
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
		return strings.TrimRight(arg, "/"), nil
	}

	// Treat as server name
	store, err := localstore.NewStore()
	if err != nil {
		return "", err
	}
	idx, err := store.LoadIndex()
	if err != nil {
		return "", err
	}
	url, ok := idx.Servers[arg]
	if !ok {
		return "", fmt.Errorf("'%s' is not a configured server; run 'nebi server add %s <url>' first", arg, arg)
	}
	return strings.TrimRight(url, "/"), nil
}

func runLogin(cmd *cobra.Command, args []string) error {
	serverURL, err := resolveServerURL(args[0])
	if err != nil {
		return err
	}

	var token string
	var username string

	if loginToken != "" {
		// Token-based login — just store it
		token = loginToken
		username = "(token)"
	} else {
		// Interactive username/password login
		fmt.Print("Username: ")
		var user string
		if _, err := fmt.Scanln(&user); err != nil {
			return fmt.Errorf("reading username: %w", err)
		}

		fmt.Print("Password: ")
		passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}

		client := cliclient.NewWithoutAuth(serverURL)
		resp, err := client.Login(context.Background(), user, string(passBytes))
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		token = resp.Token
		username = user
	}

	creds, err := localstore.LoadCredentials()
	if err != nil {
		return err
	}

	creds.Servers[serverURL] = &localstore.ServerCredential{
		Token:    token,
		Username: username,
	}

	if err := localstore.SaveCredentials(creds); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Logged in to %s as %s\n", serverURL, username)
	return nil
}
