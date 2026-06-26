package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newDiscoveryServer starts a test server that serves an OIDC discovery
// document advertising `issuer` regardless of the URL used to reach it. This
// mimics a Keycloak that is reachable in-cluster at one URL while its tokens
// carry the external (public) issuer.
func newDiscoveryServer(t *testing.T, issuer string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/protocol/openid-connect/auth",
			"token_endpoint":                        srv.URL + "/protocol/openid-connect/token",
			"jwks_uri":                              srv.URL + "/protocol/openid-connect/certs",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	})
	t.Cleanup(srv.Close)
	return srv
}

// TestNewOIDCAuthenticator_DiscoveryURLAllowsIssuerMismatch verifies the
// split-horizon case: discovery is fetched from an in-cluster URL while the
// document advertises a different (public) issuer. Setting DiscoveryURL must
// make this succeed; the issuer is still kept for "iss" validation.
func TestNewOIDCAuthenticator_DiscoveryURLAllowsIssuerMismatch(t *testing.T) {
	const publicIssuer = "https://keycloak.example.com/realms/nebari"
	srv := newDiscoveryServer(t, publicIssuer)

	cfg := OIDCConfig{
		IssuerURL:    publicIssuer,
		DiscoveryURL: srv.URL,
		ClientID:     "nebi",
	}
	if _, err := NewOIDCAuthenticator(context.Background(), cfg, nil, "test-secret", nil); err != nil {
		t.Fatalf("expected discovery via DiscoveryURL to succeed, got error: %v", err)
	}
}

// TestNewOIDCAuthenticator_IssuerMismatchWithoutDiscoveryURLFails verifies the
// default behavior is unchanged: with no DiscoveryURL, discovery is fetched
// from IssuerURL and go-oidc still rejects an "iss" that does not match the
// URL it was fetched from.
func TestNewOIDCAuthenticator_IssuerMismatchWithoutDiscoveryURLFails(t *testing.T) {
	const publicIssuer = "https://keycloak.example.com/realms/nebari"
	srv := newDiscoveryServer(t, publicIssuer)

	cfg := OIDCConfig{
		IssuerURL: srv.URL, // document advertises publicIssuer, not srv.URL
		ClientID:  "nebi",
	}
	if _, err := NewOIDCAuthenticator(context.Background(), cfg, nil, "test-secret", nil); err == nil {
		t.Fatal("expected issuer mismatch to fail without DiscoveryURL, got nil error")
	}
}
