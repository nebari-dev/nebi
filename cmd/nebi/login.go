package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
)

var loginCmd = &cobra.Command{
	Use:   "login <server-url>",
	Short: "Connect to a nebi server",
	Long: `Sets the server URL and authenticates with a nebi server.

Examples:
  # Default: device flow via Keycloak (opens browser, works with proxy)
  nebi login https://nebi.company.com

  # Username/password login
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
	loginCmd.Flags().StringVarP(&loginUsername, "username", "u", "", "Username/password login (prompts for password)")
	loginCmd.Flags().BoolVar(&loginPasswordStdin, "password-stdin", false, "Read password from stdin (requires --username)")
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
		t, u, err := usernamePasswordLogin(serverURL, loginUsername)
		if err != nil {
			return err
		}
		token = t
		username = u
	} else {
		// Default: try device flow, fall back to username/password prompt
		t, u, err := interactiveLogin(serverURL)
		if err != nil {
			return err
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

// interactiveLogin tries RFC 8628 device flow first, falls back to username/password.
func interactiveLogin(serverURL string) (token, username string, err error) {
	ctx := context.Background()
	client := cliclient.NewWithoutAuth(serverURL)

	// Check if the server supports device flow
	deviceCfg, err := client.GetDeviceConfig(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not check device flow support: %v\n", err)
		fmt.Fprintf(os.Stderr, "Falling back to username/password login.\n\n")
		return promptUsernamePasswordLogin(serverURL)
	}

	if !deviceCfg.Enabled {
		fmt.Fprintf(os.Stderr, "Server does not support device flow.\n")
		return promptUsernamePasswordLogin(serverURL)
	}

	return deviceFlowLogin(ctx, serverURL, client, deviceCfg)
}

// deviceFlowLogin performs OAuth2 Device Authorization Grant (RFC 8628) via Keycloak.
func deviceFlowLogin(ctx context.Context, serverURL string, client *cliclient.Client, cfg *cliclient.DeviceConfigResponse) (token, username string, err error) {
	// Discover the device authorization endpoint from OIDC well-known config
	deviceAuthURL, tokenURL, err := discoverDeviceEndpoints(ctx, cfg.IssuerURL)
	if err != nil {
		return "", "", fmt.Errorf("OIDC discovery failed: %w", err)
	}

	// Request device authorization from Keycloak
	deviceResp, err := requestDeviceAuthorization(ctx, deviceAuthURL, cfg.ClientID)
	if err != nil {
		return "", "", fmt.Errorf("device authorization failed: %w", err)
	}

	// Show the user code and verification URI
	fmt.Fprintf(os.Stderr, "To authenticate, open the following URL in your browser:\n\n")
	fmt.Fprintf(os.Stderr, "  %s\n\n", deviceResp.VerificationURIComplete)
	fmt.Fprintf(os.Stderr, "And verify the code: %s\n\n", deviceResp.UserCode)

	// Try to open the browser
	if err := openBrowser(deviceResp.VerificationURIComplete); err != nil {
		fmt.Fprintf(os.Stderr, "(Could not open browser automatically)\n\n")
	}

	fmt.Fprintf(os.Stderr, "Waiting for authentication...\n")

	// Poll Keycloak's token endpoint
	interval := deviceResp.Interval
	if interval < 5 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second)

	var idToken string
	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)

		tokenResp, pollErr := pollDeviceToken(ctx, tokenURL, cfg.ClientID, deviceResp.DeviceCode)
		if pollErr != nil {
			if pollErr == errAuthorizationPending || pollErr == errSlowDown {
				if pollErr == errSlowDown {
					interval += 5
				}
				continue
			}
			return "", "", fmt.Errorf("token polling failed: %w", pollErr)
		}

		idToken = tokenResp.IDToken
		break
	}

	if idToken == "" {
		return "", "", fmt.Errorf("timed out waiting for authentication")
	}

	// Exchange the Keycloak ID token for a Nebi JWT
	exchangeResp, err := client.ExchangeDeviceToken(ctx, idToken)
	if err != nil {
		return "", "", fmt.Errorf("token exchange failed: %w", err)
	}

	return exchangeResp.Token, exchangeResp.Username, nil
}

// oidcDiscovery is a subset of the OIDC well-known configuration.
type oidcDiscovery struct {
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

// discoverDeviceEndpoints fetches the OIDC well-known configuration to find
// the device authorization and token endpoints.
func discoverDeviceEndpoints(ctx context.Context, issuerURL string) (deviceAuthURL, tokenURL string, err error) {
	wellKnown := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("OIDC discovery returned %d", resp.StatusCode)
	}

	var disc oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		return "", "", err
	}

	if disc.DeviceAuthorizationEndpoint == "" {
		return "", "", fmt.Errorf("OIDC provider does not support device authorization grant")
	}

	return disc.DeviceAuthorizationEndpoint, disc.TokenEndpoint, nil
}

// deviceAuthResponse is the response from the device authorization endpoint.
type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// requestDeviceAuthorization sends a device authorization request to Keycloak.
func requestDeviceAuthorization(ctx context.Context, deviceAuthURL, clientID string) (*deviceAuthResponse, error) {
	data := url.Values{
		"client_id": {clientID},
		"scope":     {"openid profile email groups"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device authorization returned %d", resp.StatusCode)
	}

	var result deviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

var (
	errAuthorizationPending = fmt.Errorf("authorization_pending")
	errSlowDown             = fmt.Errorf("slow_down")
)

// deviceTokenResponse is the successful response from the token endpoint.
type deviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
}

// tokenErrorResponse is the error response from the token endpoint.
type tokenErrorResponse struct {
	Error string `json:"error"`
}

// pollDeviceToken polls Keycloak's token endpoint for the device code grant.
func pollDeviceToken(ctx context.Context, tokenURL, clientID, deviceCode string) (*deviceTokenResponse, error) {
	data := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {clientID},
		"device_code": {deviceCode},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var result deviceTokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		return &result, nil
	}

	// Handle RFC 8628 error codes
	var errResp tokenErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return nil, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	switch errResp.Error {
	case "authorization_pending":
		return nil, errAuthorizationPending
	case "slow_down":
		return nil, errSlowDown
	case "expired_token":
		return nil, fmt.Errorf("device code expired — please try again")
	case "access_denied":
		return nil, fmt.Errorf("access denied — user declined authorization")
	default:
		return nil, fmt.Errorf("token error: %s", errResp.Error)
	}
}

// promptUsernamePasswordLogin prompts for username and password interactively.
func promptUsernamePasswordLogin(serverURL string) (token, username string, err error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", "", fmt.Errorf("device flow not available and stdin is not a terminal; use --username and --password-stdin")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, "Username: ")
	username, _ = reader.ReadString('\n')
	username = strings.TrimSpace(username)
	if username == "" {
		return "", "", fmt.Errorf("username cannot be empty")
	}

	return usernamePasswordLogin(serverURL, username)
}

// usernamePasswordLogin authenticates with username and password.
func usernamePasswordLogin(serverURL, username string) (token, user string, err error) {
	var password string

	if loginPasswordStdin {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			password = scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			return "", "", fmt.Errorf("reading password from stdin: %w", err)
		}
	} else if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Password: ")
		passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", "", fmt.Errorf("reading password: %w", err)
		}
		password = string(passBytes)
	} else {
		return "", "", fmt.Errorf("password required: use --password-stdin for non-interactive input")
	}

	client := cliclient.NewWithoutAuth(serverURL)
	resp, err := client.Login(context.Background(), username, password)
	if err != nil {
		return "", "", fmt.Errorf("login failed: %w", err)
	}

	return resp.Token, username, nil
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
