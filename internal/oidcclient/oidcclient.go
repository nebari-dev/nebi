// Package oidcclient implements the client side of nebi's OIDC login: the
// RFC 8628 device authorization grant used by the CLI and the desktop app,
// and a refreshing token source for the stored tokens. Tokens are issued by
// the identity provider; the nebi server only validates them. Discovery,
// the device flow, PKCE and token refresh are delegated to go-oidc and
// golang.org/x/oauth2; this package only adds nebi's defaults and errors.
package oidcclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OfflineAccessScope asks the identity provider for a refresh token that is
// not bound to a browser SSO session, so CLI and desktop logins survive the
// provider's session idle timeout.
const OfflineAccessScope = "offline_access"

var (
	// ErrExpired means the device code expired before the user approved it.
	ErrExpired = errors.New("device code expired, please try again")
	// ErrAccessDenied means the user declined the authorization request.
	ErrAccessDenied = errors.New("access denied, the authorization request was declined")
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// withHTTPClient makes go-oidc and oauth2 use the bounded httpClient instead
// of http.DefaultClient, which has no timeout.
func withHTTPClient(ctx context.Context) context.Context {
	return oidc.ClientContext(ctx, httpClient)
}

// Discover reads the provider's OpenID configuration and returns the
// configuration of nebi's public client for the device flow.
func Discover(ctx context.Context, issuerURL, clientID string, scopes []string) (*oauth2.Config, error) {
	provider, err := oidc.NewProvider(withHTTPClient(ctx), issuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	endpoint := provider.Endpoint()
	if endpoint.DeviceAuthURL == "" {
		return nil, errors.New("OIDC provider does not support the device authorization grant")
	}
	// nebi's client is public: send client_id in the form instead of letting
	// oauth2 probe for HTTP basic auth with an empty secret.
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	return &oauth2.Config{ClientID: clientID, Endpoint: endpoint, Scopes: scopes}, nil
}

// DeviceAuthorization is a pending device authorization and the PKCE
// verifier (RFC 7636) that the token request for it must present.
type DeviceAuthorization struct {
	oauth2.DeviceAuthResponse
	Verifier string
}

// BrowseURL returns the URL the user should open to approve the device.
// verification_uri_complete is optional per RFC 8628 §3.2.
func (d *DeviceAuthorization) BrowseURL() string {
	if d.VerificationURIComplete != "" {
		return d.VerificationURIComplete
	}
	return d.VerificationURI
}

// StartDeviceAuthorization requests a device and user code. The request
// carries a PKCE challenge, which providers may require for public clients
// such as the CLI.
func StartDeviceAuthorization(ctx context.Context, cfg *oauth2.Config) (*DeviceAuthorization, error) {
	verifier := oauth2.GenerateVerifier()
	resp, err := cfg.DeviceAuth(withHTTPClient(ctx), oauth2.S256ChallengeOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("device authorization: %w", err)
	}
	if resp.DeviceCode == "" || resp.UserCode == "" {
		return nil, errors.New("device authorization response is missing the device or user code")
	}
	// expires_in is required (RFC 8628 §3.2) and is what bounds polling in
	// WaitForDeviceToken; without it the wait would only end on cancellation.
	if resp.Expiry.IsZero() {
		return nil, errors.New("device authorization response has no expires_in")
	}
	return &DeviceAuthorization{DeviceAuthResponse: *resp, Verifier: verifier}, nil
}

// WaitForDeviceToken polls until the user approves the device, the code
// expires, or ctx is done. Polling follows RFC 8628 §3.5 (interval and
// slow_down) as implemented by oauth2.Config.DeviceAccessToken.
func WaitForDeviceToken(ctx context.Context, cfg *oauth2.Config, da *DeviceAuthorization) (*oauth2.Token, error) {
	tok, err := cfg.DeviceAccessToken(withHTTPClient(ctx), &da.DeviceAuthResponse, oauth2.VerifierOption(da.Verifier))
	if err == nil {
		return tok, nil
	}
	var re *oauth2.RetrieveError
	switch {
	case errors.As(err, &re) && re.ErrorCode == "expired_token":
		return nil, ErrExpired
	case errors.As(err, &re) && re.ErrorCode == "access_denied":
		return nil, ErrAccessDenied
	case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
		// DeviceAccessToken stops polling at the device code's expiry.
		return nil, ErrExpired
	}
	return nil, err
}

// TokenSource returns a source that serves tok until it is about to expire
// and then refreshes it against tokenURL. onRefresh is called with every
// newly issued token so callers can persist it (providers may rotate the
// refresh token). Without a refresh token, tok is served as is.
func TokenSource(ctx context.Context, tokenURL, clientID string, tok *oauth2.Token, onRefresh func(*oauth2.Token) error) oauth2.TokenSource {
	if tok.RefreshToken == "" || tokenURL == "" {
		return oauth2.StaticTokenSource(tok)
	}
	cfg := &oauth2.Config{
		ClientID: clientID,
		Endpoint: oauth2.Endpoint{TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}
	return &notifyingSource{
		src:       oauth2.ReuseTokenSource(tok, cfg.TokenSource(withHTTPClient(ctx), tok)),
		last:      tok.AccessToken,
		onRefresh: onRefresh,
	}
}

type notifyingSource struct {
	src       oauth2.TokenSource
	onRefresh func(*oauth2.Token) error

	// mu guards last. ReuseTokenSource serialises the refresh itself, but
	// callers sharing one source (the desktop app serves parallel requests
	// from it) all observe the new token at once and must not race on it.
	mu   sync.Mutex
	last string
}

func (s *notifyingSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, fmt.Errorf("refreshing login (run 'nebi login' again if this persists): %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok.AccessToken != s.last {
		s.last = tok.AccessToken
		if s.onRefresh != nil {
			if err := s.onRefresh(tok); err != nil {
				return nil, fmt.Errorf("saving refreshed token: %w", err)
			}
		}
	}
	return tok, nil
}
