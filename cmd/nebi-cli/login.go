package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/oidcclient"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login <server-url>",
	Short: "Connect to a nebi server",
	Long: `Sets the server URL and authenticates with a nebi server.

nebi servers delegate authentication to an OpenID Connect provider (for
example Keycloak). By default this opens the provider's login page in your
browser (OAuth device authorization grant) and stores the resulting tokens,
which are refreshed automatically.

Examples:
  # Log in through the server's identity provider (opens a browser)
  nebi login https://nebi.company.com

  # Use an access token you obtained from the identity provider yourself.
  # It is not refreshed, so this is mostly useful for short-lived automation.
  nebi login https://nebi.company.com --token <access-token>`,
	Args: cobra.ExactArgs(1),
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "Access token issued by the server's identity provider (skips interactive login; not refreshed)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	serverURL := strings.TrimRight(args[0], "/")
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return fmt.Errorf("server URL must start with http:// or https://")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	creds := &store.Credentials{}
	if loginToken != "" {
		creds.Token = loginToken
	} else {
		var err error
		creds, err = interactiveLogin(ctx, serverURL)
		if err != nil {
			return err
		}
	}

	me, err := cliclient.New(serverURL, creds.Token).GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("verifying login with %s: %w", serverURL, err)
	}
	creds.Username = me.Username

	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.SaveServerURL(serverURL); err != nil {
		return err
	}
	if err := s.SaveCredentials(creds); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Logged in to %s as %s\n", serverURL, creds.Username)
	return nil
}

// interactiveLogin logs in through the server's identity provider with the
// OAuth 2.0 device authorization grant (RFC 8628). Servers with
// authentication disabled need no credentials.
func interactiveLogin(ctx context.Context, serverURL string) (*store.Credentials, error) {
	authCfg, err := cliclient.NewWithoutAuth(serverURL).GetAuthConfig(ctx)
	if err != nil {
		return nil, err
	}
	switch authCfg.Type {
	case cliclient.AuthTypeNone:
		fmt.Fprintf(os.Stderr, "Server %s has authentication disabled.\n", serverURL)
		return &store.Credentials{}, nil
	case cliclient.AuthTypeOIDC:
	default:
		return nil, fmt.Errorf("server uses unsupported authentication %q; upgrade nebi", authCfg.Type)
	}

	scopes := append(authCfg.Scopes, oidcclient.OfflineAccessScope)
	oauthCfg, err := oidcclient.Discover(ctx, authCfg.IssuerURL, authCfg.ClientID, scopes)
	if err != nil {
		return nil, err
	}
	device, err := oidcclient.StartDeviceAuthorization(ctx, oauthCfg)
	if err != nil {
		return nil, err
	}

	browseURL := device.BrowseURL()
	fmt.Fprintf(os.Stderr, "To authenticate, open the following URL in your browser:\n\n")
	fmt.Fprintf(os.Stderr, "  %s\n\n", browseURL)
	fmt.Fprintf(os.Stderr, "And verify the code: %s\n\n", device.UserCode)
	if err := openBrowser(browseURL); err != nil {
		fmt.Fprintf(os.Stderr, "(Could not open browser automatically)\n\n")
	}
	fmt.Fprintf(os.Stderr, "Waiting for authentication...\n")

	tok, err := oidcclient.WaitForDeviceToken(ctx, oauthCfg, device)
	if err != nil {
		return nil, fmt.Errorf("device login failed: %w", err)
	}

	creds := &store.Credentials{TokenURL: oauthCfg.Endpoint.TokenURL, ClientID: authCfg.ClientID}
	creds.SetOAuthToken(tok)
	return creds, nil
}

// openBrowser opens the given URL in the user's default browser. It is a
// variable so tests can stub it.
var openBrowser = func(url string) error {
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
