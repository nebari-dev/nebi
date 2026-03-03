package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	loginToken         string
	loginUsername      string
	loginPasswordStdin bool
	loginBrowser       bool
)

var loginCmd = &cobra.Command{
	Use:   "login <server-url>",
	Short: "Connect to a nebi server",
	Long: `Sets the server URL and authenticates with a nebi server.

Examples:
  # Interactive - prompts for username and password
  nebi login https://nebi.company.com

  # Browser-based login (required when behind an OIDC proxy like Keycloak)
  nebi login https://nebi.company.com --browser

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
	loginCmd.Flags().BoolVar(&loginBrowser, "browser", false, "Login via browser (for servers behind OIDC proxy)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	serverURL := strings.TrimRight(args[0], "/")

	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return fmt.Errorf("server URL must start with http:// or https://")
	}

	// Validate flag combinations
	if loginBrowser && loginToken != "" {
		return fmt.Errorf("cannot use --browser with --token")
	}
	if loginBrowser && loginPasswordStdin {
		return fmt.Errorf("cannot use --browser with --password-stdin")
	}
	if loginPasswordStdin && loginToken != "" {
		return fmt.Errorf("cannot use --password-stdin with --token")
	}
	if loginPasswordStdin && loginUsername == "" {
		return fmt.Errorf("--password-stdin requires --username")
	}

	var token string
	var username string

	if loginBrowser {
		// Browser-based login for servers behind OIDC proxies
		t, u, err := browserLogin(serverURL)
		if err != nil {
			return fmt.Errorf("browser login failed: %w", err)
		}
		token = t
		username = u
	} else if loginToken != "" {
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
			// Detect OIDC proxy redirect: the server returned HTML instead of JSON
			if cliclient.IsOIDCRedirect(err) {
				return fmt.Errorf("login failed: server appears to be behind an OIDC proxy (Keycloak/Envoy).\nUse browser-based login instead:\n\n  nebi login %s --browser", serverURL)
			}
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

// browserLogin performs browser-based authentication for servers behind OIDC proxies.
// It starts a local HTTP server, opens the browser to the server's CLI login endpoint,
// and waits for the OIDC-authenticated callback with a Nebi JWT.
func browserLogin(serverURL string) (token, username string, err error) {
	// Start a local callback server on a random port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", "", fmt.Errorf("failed to start local callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Channel to receive the auth result
	type authResult struct {
		token    string
		username string
		err      error
	}
	resultCh := make(chan authResult, 1)

	// Set up the callback handler
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		t := r.URL.Query().Get("token")
		u := r.URL.Query().Get("username")
		if t == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.WriteHeader(http.StatusOK)
			resultCh <- authResult{err: fmt.Errorf("no token received from server")}
			return
		}

		// Allow cross-origin requests from the server page
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
		resultCh <- authResult{token: t, username: u}
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Close()

	// Build the login URL
	loginURL := fmt.Sprintf("%s/api/v1/auth/cli-login?callback_port=%d", serverURL, port)

	fmt.Fprintf(os.Stderr, "Opening browser for authentication...\n")
	fmt.Fprintf(os.Stderr, "If the browser doesn't open, visit:\n  %s\n\n", loginURL)
	fmt.Fprintf(os.Stderr, "Waiting for authentication...\n")

	// Open browser
	if err := openBrowser(loginURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
	}

	// Wait for callback with timeout
	select {
	case result := <-resultCh:
		if result.err != nil {
			return "", "", result.err
		}
		return result.token, result.username, nil
	case <-time.After(5 * time.Minute):
		return "", "", fmt.Errorf("timed out waiting for browser authentication (5 minutes)")
	}
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
