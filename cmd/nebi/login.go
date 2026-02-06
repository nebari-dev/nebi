package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login <server-url>",
	Short: "Connect to a nebi server",
	Long: `Sets the server URL and authenticates with a nebi server.

Examples:
  nebi login https://nebi.company.com
  nebi login https://nebi.company.com --token <api-token>`,
	Args: cobra.ExactArgs(1),
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "API token (skip interactive login)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	serverURL := strings.TrimRight(args[0], "/")

	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return fmt.Errorf("server URL must start with http:// or https://")
	}

	var token string
	var username string

	if loginToken != "" {
		token = loginToken
		username = "(token)"
	} else {
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

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.SaveServerURL(serverURL); err != nil {
		return err
	}

	if err := s.SaveCredentials(&store.Credentials{Token: token, Username: username}); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Logged in to %s as %s\n", serverURL, username)
	return nil
}
