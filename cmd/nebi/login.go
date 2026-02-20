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

By default, opens a browser for authentication (works with proxy/Keycloak
deployments). Use --username for password-based login or --token for direct
token authentication.

Examples:
  # Browser login (default) — works with proxy/Keycloak deployments
  # Opens a browser; also works over SSH (prints URL to open on any device)
  nebi login https://nebi.company.com

  # Password login — prompts for username and password
  nebi login https://nebi.company.com --username myuser

  # Non-interactive with password from stdin
  echo "$PASSWORD" | nebi login https://nebi.company.com --username myuser --password-stdin

  # Using an API token (skips interactive login)
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
		// Direct token mode
		token = loginToken
		username = "(token)"
	} else if loginUsername != "" {
		// Username/password mode
		var password string

		if loginPasswordStdin {
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
		} else {
			return fmt.Errorf("password required: use --password-stdin for non-interactive input")
		}

		client := cliclient.NewWithoutAuth(serverURL)
		resp, err := client.Login(context.Background(), loginUsername, password)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		token = resp.Token
		username = loginUsername
	} else {
		// Browser-based login (default)
		t, u, err := browserLogin(context.Background(), serverURL)
		if err != nil {
			return fmt.Errorf("browser login failed: %w", err)
		}
		token = t
		username = u
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
