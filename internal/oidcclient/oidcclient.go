// Package oidcclient implements the client side of nebi's OIDC login: the
// RFC 8628 device authorization grant used by the CLI and the desktop app,
// and a refreshing token source for the stored tokens. Tokens are issued by
// the identity provider; the nebi server only validates them.
package oidcclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// OfflineAccessScope asks the identity provider for a refresh token that is
// not bound to a browser SSO session, so CLI and desktop logins survive the
// provider's session idle timeout.
const OfflineAccessScope = "offline_access"

var (
	// ErrAuthorizationPending means the user has not approved the device yet.
	ErrAuthorizationPending = errors.New("authorization pending")
	// ErrSlowDown means the client must increase its polling interval.
	ErrSlowDown = errors.New("slow down")
	// ErrExpired means the device code expired before the user approved it.
	ErrExpired = errors.New("device code expired, please try again")
	// ErrAccessDenied means the user declined the authorization request.
	ErrAccessDenied = errors.New("access denied, the authorization request was declined")
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Endpoints are the provider endpoints a device-flow client needs.
type Endpoints struct {
	DeviceAuthorization string `json:"device_authorization_endpoint"`
	Token               string `json:"token_endpoint"`
}

// Discover reads the provider's OpenID configuration.
func Discover(ctx context.Context, issuerURL string) (*Endpoints, error) {
	wellKnown := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC discovery returned %d", resp.StatusCode)
	}

	var ep Endpoints
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return nil, fmt.Errorf("decode OIDC discovery: %w", err)
	}
	if ep.Token == "" {
		return nil, errors.New("OIDC provider does not advertise a token endpoint")
	}
	if ep.DeviceAuthorization == "" {
		return nil, errors.New("OIDC provider does not support the device authorization grant")
	}
	return &ep, nil
}

// DeviceAuthorization is the provider's answer to a device authorization request.
type DeviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`

	// CodeVerifier is the PKCE verifier bound to this authorization. It is
	// sent with every token request for the device code.
	CodeVerifier string `json:"-"`
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
// carries a PKCE challenge (RFC 7636), which providers may require for public
// clients such as the CLI.
func StartDeviceAuthorization(ctx context.Context, ep *Endpoints, clientID string, scopes []string) (*DeviceAuthorization, error) {
	verifier := oauth2.GenerateVerifier()
	form := url.Values{
		"client_id":             {clientID},
		"scope":                 {strings.Join(scopes, " ")},
		"code_challenge":        {oauth2.S256ChallengeFromVerifier(verifier)},
		"code_challenge_method": {"S256"},
	}
	body, status, err := postForm(ctx, ep.DeviceAuthorization, form)
	if err != nil {
		return nil, fmt.Errorf("device authorization: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("device authorization returned %d: %s", status, providerError(body))
	}

	var da DeviceAuthorization
	if err := json.Unmarshal(body, &da); err != nil {
		return nil, fmt.Errorf("decode device authorization: %w", err)
	}
	if da.DeviceCode == "" || da.UserCode == "" {
		return nil, errors.New("device authorization response is missing the device or user code")
	}
	if da.Interval <= 0 {
		da.Interval = 5
	}
	da.CodeVerifier = verifier
	return &da, nil
}

// PollDeviceToken makes one token request for a pending device
// authorization. It returns ErrAuthorizationPending or ErrSlowDown while the
// user has not approved yet, and ErrExpired or ErrAccessDenied when the flow
// is over.
func PollDeviceToken(ctx context.Context, ep *Endpoints, clientID string, da *DeviceAuthorization) (*oauth2.Token, error) {
	form := url.Values{
		"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":     {clientID},
		"device_code":   {da.DeviceCode},
		"code_verifier": {da.CodeVerifier},
	}
	body, status, err := postForm(ctx, ep.Token, form)
	if err != nil {
		return nil, fmt.Errorf("device token: %w", err)
	}
	if status != http.StatusOK {
		switch code := providerErrorCode(body); code {
		case "authorization_pending":
			return nil, ErrAuthorizationPending
		case "slow_down":
			return nil, ErrSlowDown
		case "expired_token":
			return nil, ErrExpired
		case "access_denied":
			return nil, ErrAccessDenied
		default:
			return nil, fmt.Errorf("token endpoint returned %d: %s", status, providerError(body))
		}
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decode device token: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("token response has no access token")
	}
	tok := &oauth2.Token{
		AccessToken:  tr.AccessToken,
		TokenType:    tr.TokenType,
		RefreshToken: tr.RefreshToken,
	}
	if tr.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// WaitForDeviceToken polls until the user approves the device, the code
// expires, or ctx is done.
func WaitForDeviceToken(ctx context.Context, ep *Endpoints, clientID string, da *DeviceAuthorization) (*oauth2.Token, error) {
	interval := time.Duration(da.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(da.ExpiresIn) * time.Second)
	for da.ExpiresIn <= 0 || time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		tok, err := PollDeviceToken(ctx, ep, clientID, da)
		switch {
		case errors.Is(err, ErrAuthorizationPending):
			continue
		case errors.Is(err, ErrSlowDown):
			interval += 5 * time.Second
			continue
		case err != nil:
			return nil, err
		}
		return tok, nil
	}
	return nil, ErrExpired
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
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	return &notifyingSource{
		src:       oauth2.ReuseTokenSource(tok, cfg.TokenSource(ctx, tok)),
		last:      tok.AccessToken,
		onRefresh: onRefresh,
	}
}

type notifyingSource struct {
	src       oauth2.TokenSource
	last      string
	onRefresh func(*oauth2.Token) error
}

func (s *notifyingSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, fmt.Errorf("refreshing login (run 'nebi login' again if this persists): %w", err)
	}
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

func postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func providerErrorCode(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	return e.Error
}

func providerError(body []byte) string {
	var e struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error == "" {
		return strings.TrimSpace(string(body))
	}
	if e.Description != "" {
		return e.Error + ": " + e.Description
	}
	return e.Error
}
