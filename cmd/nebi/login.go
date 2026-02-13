package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	loginToken         string
	loginUsername      string
	loginPasswordStdin bool
)

var loginCmd = &cobra.Command{
	Use:   "login <server-url>",
	Short: "Connect to a nebi server",
	Long: `Sets the server URL and authenticates with a nebi server.

Examples:
  # Interactive - prompts for username and password
  nebi login https://nebi.company.com

  # Non-interactive with username flag and password from stdin
  echo "$PASSWORD" | nebi login https://nebi.company.com --username myuser --password-stdin

  # Using an API token (skips username/password)
  nebi login https://nebi.company.com --token <api-token>`,
	Args: cobra.ExactArgs(1),
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "API token (skip interactive login)")
	loginCmd.Flags().StringVarP(&loginUsername, "username", "u", "", "Username for authentication")
	loginCmd.Flags().BoolVar(&loginPasswordStdin, "password-stdin", false, "Read password from stdin")
}

func runLogin(cmd *cobra.Command, args []string) error {
	serverURL := strings.TrimRight(args[0], "/")

	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return fmt.Errorf("server URL must start with http:// or https://")
	}

	// Validate flag combinations
	if loginPasswordStdin && loginToken != "" {
		return fmt.Errorf("cannot use --password-stdin with --token")
	}
	if loginPasswordStdin && loginUsername == "" {
		return fmt.Errorf("--password-stdin requires --username")
	}

	var token string
	var username string

	if loginToken != "" {
		token = loginToken
		username = "(token)"
	} else {
		var user string
		var password string

		// Get username
		if loginUsername != "" {
			user = loginUsername
		} else {
			fmt.Fprint(os.Stderr, "Username: ")
			if _, err := fmt.Scanln(&user); err != nil {
				return fmt.Errorf("reading username: %w", err)
			}
		}

		// Get password
		if loginPasswordStdin {
			// Read password from stdin (for scripting)
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
		} else {
			return fmt.Errorf("password required: use --password-stdin for non-interactive input")
		}

		client := cliclient.NewWithoutAuth(serverURL)
		resp, err := client.Login(context.Background(), user, password)
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
