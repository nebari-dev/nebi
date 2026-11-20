// Package authtest runs an in-process OIDC provider for tests. It signs JWT
// access tokens for arbitrary identities and implements just enough of the
// token and device authorization endpoints for the CLI and desktop login
// flows. Test code only: never import it from production code.
package authtest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"golang.org/x/oauth2"
)

const keyID = "authtest-key"

// Identity is the user a token is issued for.
type Identity struct {
	Subject       string
	Username      string
	Email         string
	EmailVerified bool
	Name          string
	Groups        []string
}

// Server is a fake OIDC provider.
type Server struct {
	URL      string // issuer URL
	ClientID string // audience placed in every token

	srv  *httptest.Server
	priv *rsa.PrivateKey
	keys *oidctest.Server

	mu      sync.Mutex
	devices map[string]*deviceGrant
	refresh map[string]Identity
	nextID  atomic.Int64

	// TokenTTL is the lifetime of issued access tokens (default 5 minutes).
	TokenTTL time.Duration

	// AutoApprove, when set, approves every new device authorization as
	// this identity, standing in for the user confirming in a browser.
	AutoApprove *Identity
}

type deviceGrant struct {
	identity  *Identity // nil until approved
	denied    bool
	challenge string // PKCE S256 challenge from the device request
}

// NewServer starts a provider whose tokens carry aud=clientID.
func NewServer(clientID string) (*Server, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}
	s := &Server{
		ClientID: clientID,
		priv:     priv,
		keys: &oidctest.Server{PublicKeys: []oidctest.PublicKey{{
			PublicKey: priv.Public(),
			KeyID:     keyID,
			Algorithm: oidc.RS256,
		}}},
		devices:  map[string]*deviceGrant{},
		refresh:  map[string]Identity{},
		TokenTTL: 5 * time.Minute,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.serveDiscovery)
	mux.Handle("/keys", s.keys)
	mux.HandleFunc("/token", s.serveToken)
	mux.HandleFunc("/device", s.serveDevice)
	s.srv = httptest.NewServer(mux)
	s.URL = s.srv.URL
	s.keys.SetIssuer(s.URL)
	return s, nil
}

// Close stops the provider.
func (s *Server) Close() { s.srv.Close() }

// Token signs an access token for id that expires after TokenTTL.
func (s *Server) Token(id Identity) string {
	return s.TokenWithClaims(id, nil)
}

// TokenWithClaims signs an access token for id with extra or overriding claims.
func (s *Server) TokenWithClaims(id Identity, extra map[string]any) string {
	now := time.Now()
	claims := map[string]any{
		"iss":                s.URL,
		"aud":                s.ClientID,
		"sub":                id.Subject,
		"iat":                now.Unix(),
		"exp":                now.Add(s.TokenTTL).Unix(),
		"jti":                fmt.Sprintf("jti-%d", s.nextID.Add(1)),
		"preferred_username": id.Username,
		"email":              id.Email,
		"email_verified":     id.EmailVerified,
		"name":               id.Name,
		"groups":             id.Groups,
	}
	for k, v := range extra {
		claims[k] = v
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	return oidctest.SignIDToken(s.priv, keyID, oidc.RS256, string(raw))
}

// ApproveDevice approves the pending device authorization for userCode as id.
func (s *Server) ApproveDevice(userCode string, id Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.devices[userCode]
	if !ok {
		return fmt.Errorf("unknown user code %q", userCode)
	}
	g.identity = &id
	return nil
}

// DenyDevice declines the pending device authorization for userCode.
func (s *Server) DenyDevice(userCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.devices[userCode]
	if !ok {
		return fmt.Errorf("unknown user code %q", userCode)
	}
	g.denied = true
	return nil
}

func (s *Server) serveDiscovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.URL,
		"authorization_endpoint":                s.URL + "/auth",
		"token_endpoint":                        s.URL + "/token",
		"device_authorization_endpoint":         s.URL + "/device",
		"end_session_endpoint":                  s.URL + "/logout",
		"jwks_uri":                              s.URL + "/keys",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{oidc.RS256},
	})
}

// serveDevice issues device codes. The user code doubles as the key for
// ApproveDevice and DenyDevice, and the device code is "device-<user code>".
func (s *Server) serveDevice(w http.ResponseWriter, r *http.Request) {
	if r.PostFormValue("client_id") == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client"})
		return
	}
	// Like Keycloak with PKCE enforced on the client, require a challenge.
	if r.PostFormValue("code_challenge_method") != "S256" || r.PostFormValue("code_challenge") == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "error_description": "Missing parameter: code_challenge_method"})
		return
	}
	userCode := fmt.Sprintf("CODE-%04d", s.nextID.Add(1))
	s.mu.Lock()
	s.devices[userCode] = &deviceGrant{identity: s.AutoApprove, challenge: r.PostFormValue("code_challenge")}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               "device-" + userCode,
		"user_code":                 userCode,
		"verification_uri":          s.URL + "/activate",
		"verification_uri_complete": s.URL + "/activate?user_code=" + userCode,
		"expires_in":                600,
		"interval":                  1,
	})
}

func (s *Server) serveToken(w http.ResponseWriter, r *http.Request) {
	switch r.PostFormValue("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		userCode := r.PostFormValue("device_code")
		if len(userCode) > len("device-") {
			userCode = userCode[len("device-"):]
		}
		s.mu.Lock()
		g, ok := s.devices[userCode]
		s.mu.Unlock()
		switch {
		case !ok:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_token"})
		case oauth2.S256ChallengeFromVerifier(r.PostFormValue("code_verifier")) != g.challenge:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "PKCE verification failed"})
		case g.denied:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "access_denied"})
		case g.identity == nil:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authorization_pending"})
		default:
			s.mu.Lock()
			delete(s.devices, userCode)
			s.mu.Unlock()
			s.writeTokens(w, *g.identity)
		}
	case "refresh_token":
		s.mu.Lock()
		id, ok := s.refresh[r.PostFormValue("refresh_token")]
		delete(s.refresh, r.PostFormValue("refresh_token"))
		s.mu.Unlock()
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		s.writeTokens(w, id)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
	}
}

// writeTokens issues an access token and a rotated refresh token.
func (s *Server) writeTokens(w http.ResponseWriter, id Identity) {
	refresh := fmt.Sprintf("refresh-%d", s.nextID.Add(1))
	s.mu.Lock()
	s.refresh[refresh] = id
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  s.Token(id),
		"token_type":    "Bearer",
		"refresh_token": refresh,
		"expires_in":    int(s.TokenTTL.Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
