package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
)

// ServerRemote implements the dynamic half of the auth split: the RFC
// 8628 device authorization grant, mirroring cmd/nebi's interactive
// login (login.go) but reshaped into the Begin/Complete pair so a
// client owns the "show the code to the user" step. OCIRemote has no
// counterpart — registries have no interactive flow — which is exactly
// what the optional extension interface expresses.
var _ DeviceAuthenticator = (*ServerRemote)(nil)

// deviceHTTPClient talks to the OIDC issuer (not the nebi server).
var deviceHTTPClient = &http.Client{Timeout: 30 * time.Second}

// BeginDeviceAuthentication asks the server for its device flow
// configuration, discovers the issuer's endpoints, and starts an
// authorization. The returned prompt must be shown to the user before
// calling CompleteDeviceAuthentication.
func (r *ServerRemote) BeginDeviceAuthentication(ctx context.Context) (DeviceAuthorization, error) {
	cfg, err := cliclient.NewWithoutAuth(r.BaseURL).GetDeviceConfig(ctx)
	if err != nil {
		return DeviceAuthorization{}, mapServerError(err)
	}
	if !cfg.Enabled {
		return DeviceAuthorization{}, errors.New("server remote: device flow not enabled on this server")
	}

	deviceAuthURL, tokenURL, err := discoverDeviceEndpoints(ctx, cfg.IssuerURL)
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("server remote: OIDC discovery: %w", err)
	}

	resp, err := requestDeviceAuthorization(ctx, deviceAuthURL, cfg.ClientID)
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("server remote: device authorization: %w", err)
	}

	interval := time.Duration(resp.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second // RFC 8628 §3.2 default
	}
	return DeviceAuthorization{
		UserCode:                resp.UserCode,
		VerificationURI:         resp.VerificationURI,
		VerificationURIComplete: resp.VerificationURIComplete,
		ExpiresAt:               time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		deviceCode:              resp.DeviceCode,
		tokenURL:                tokenURL,
		clientID:                cfg.ClientID,
		interval:                interval,
	}, nil
}

// CompleteDeviceAuthentication polls the issuer until the user approves
// the prompt, then exchanges the resulting ID token for a nebi JWT.
func (r *ServerRemote) CompleteDeviceAuthentication(ctx context.Context, da DeviceAuthorization) error {
	if da.deviceCode == "" {
		return errors.New("server remote: authorization not started (call BeginDeviceAuthentication first)")
	}
	interval := da.interval
	if r.DevicePollInterval > 0 {
		interval = r.DevicePollInterval
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		if !da.ExpiresAt.IsZero() && time.Now().After(da.ExpiresAt) {
			return errors.New("server remote: device authorization expired before the user approved it")
		}

		tok, err := pollDeviceToken(ctx, da.tokenURL, da.clientID, da.deviceCode)
		switch {
		case errors.Is(err, errAuthorizationPending):
			continue
		case errors.Is(err, errSlowDown):
			interval += 5 * time.Second
			continue
		case err != nil:
			return fmt.Errorf("server remote: device token poll: %w", err)
		}

		resp, err := cliclient.NewWithoutAuth(r.BaseURL).ExchangeDeviceToken(ctx, tok.IDToken)
		if err != nil {
			return mapServerError(err)
		}
		r.client = cliclient.New(r.BaseURL, resp.Token)
		return nil
	}
}

// The helpers below are lifted from cmd/nebi/login.go with the terminal
// I/O removed. A real implementation would share them (they belong in a
// package both the CLI and the remote layer can import).

// oidcDiscovery is the subset of the OIDC well-known configuration the
// device flow needs.
type oidcDiscovery struct {
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

func discoverDeviceEndpoints(ctx context.Context, issuerURL string) (deviceAuthURL, tokenURL string, err error) {
	wellKnown := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := deviceHTTPClient.Do(req)
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
		return "", "", errors.New("OIDC provider does not support the device authorization grant")
	}
	return disc.DeviceAuthorizationEndpoint, disc.TokenEndpoint, nil
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

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
	resp, err := deviceHTTPClient.Do(req)
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
	errAuthorizationPending = errors.New("authorization_pending")
	errSlowDown             = errors.New("slow_down")
)

type deviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
}

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
	resp, err := deviceHTTPClient.Do(req)
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

	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return nil, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}
	switch errResp.Error {
	case "authorization_pending":
		return nil, errAuthorizationPending
	case "slow_down":
		return nil, errSlowDown
	case "expired_token":
		return nil, errors.New("device code expired")
	case "access_denied":
		return nil, errors.New("user declined authorization")
	default:
		return nil, fmt.Errorf("token error: %s", errResp.Error)
	}
}
