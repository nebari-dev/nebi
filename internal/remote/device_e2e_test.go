//go:build e2e

package remote

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
)

const deviceTestClientID = "nebi-remote-spike-client"

// fakeIssuer is an in-process OIDC provider good enough for the device
// flow: go-oidc's oidctest server supplies discovery + JWKS (so the
// nebi server can verify the id_token we mint), and two extra handlers
// supply the RFC 8628 device authorization and token endpoints.
type fakeIssuer struct {
	url      string
	rawToken string
	polls    atomic.Int32 // token endpoint hits; first one reports pending
}

func startFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	oidcServer := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{{
			PublicKey: priv.Public(),
			KeyID:     "spike-key",
			Algorithm: oidc.RS256,
		}},
	}

	fi := &fakeIssuer{}
	mux := http.NewServeMux()

	// Serve oidctest's own discovery document (its jwks_uri must stay
	// intact) with the device flow endpoints spliced in.
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		oidcServer.ServeHTTP(rec, r)
		var doc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		doc["device_authorization_endpoint"] = fi.url + "/device-auth"
		doc["token_endpoint"] = fi.url + "/token"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	})

	mux.HandleFunc("/device-auth", func(w http.ResponseWriter, r *http.Request) {
		if got := r.FormValue("client_id"); got != deviceTestClientID {
			http.Error(w, "unknown client "+got, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "spike-device-code",
			"user_code":                 "ABCD-EFGH",
			"verification_uri":          fi.url + "/verify",
			"verification_uri_complete": fi.url + "/verify?user_code=ABCD-EFGH",
			"expires_in":                300,
			"interval":                  1,
		})
	})

	// First poll: the user hasn't approved yet. Every later poll
	// succeeds, simulating the browser approval happening in between.
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" ||
			r.FormValue("device_code") != "spike-device-code" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if fi.polls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "spike-access-token",
			"token_type":   "Bearer",
			"id_token":     fi.rawToken,
		})
	})

	mux.Handle("/", oidcServer)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fi.url = srv.URL
	oidcServer.SetIssuer(srv.URL)

	claims, err := json.Marshal(map[string]any{
		"iss":                srv.URL,
		"aud":                deviceTestClientID,
		"sub":                "device-flow-subject",
		"preferred_username": "device-flow-user",
		"email":              "device-flow@example.test",
		"email_verified":     true,
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	fi.rawToken = oidctest.SignIDToken(priv, "spike-key", oidc.RS256, string(claims))
	return fi
}

// TestServerRemote_DeviceFlow exercises the dynamic half of the auth
// split end to end: discovery advertises the device scheme, Begin
// returns the user prompt, Complete polls through an
// authorization_pending round, exchanges the issuer's id_token for a
// nebi JWT, and the remote comes out authenticated.
func TestServerRemote_DeviceFlow(t *testing.T) {
	issuer := startFakeIssuer(t)
	baseURL := startSpikeServer(t, map[string]string{
		"NEBI_AUTH_OIDC_ISSUER_URL":       issuer.url,
		"NEBI_AUTH_OIDC_CLIENT_ID":        deviceTestClientID,
		"NEBI_AUTH_DEVICE_FLOW_CLIENT_ID": deviceTestClientID,
	})
	ctx := context.Background()
	r := &ServerRemote{BaseURL: baseURL, DevicePollInterval: 25 * time.Millisecond}

	// The dynamic scheme is discovered, not assumed.
	schemes := r.SupportedAuthentication()
	found := false
	for _, s := range schemes {
		if s == DeviceAuthenticationCredentialType {
			found = true
		}
	}
	if !found {
		t.Fatalf("SupportedAuthentication: device scheme missing from %v", schemes)
	}

	// Client-side detection is a type assertion on the Remote value.
	var remote Remote = r
	da, ok := remote.(DeviceAuthenticator)
	if !ok {
		t.Fatal("ServerRemote does not implement DeviceAuthenticator")
	}

	auth, err := da.BeginDeviceAuthentication(ctx)
	if err != nil {
		t.Fatalf("BeginDeviceAuthentication: %v", err)
	}
	if auth.UserCode != "ABCD-EFGH" || auth.VerificationURI == "" {
		t.Fatalf("unexpected prompt: %+v", auth)
	}

	// Still unauthenticated until the flow completes.
	if _, err := remote.List(ctx); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("List mid-flow: want ErrNotAuthenticated, got %v", err)
	}

	if err := da.CompleteDeviceAuthentication(ctx, auth); err != nil {
		t.Fatalf("CompleteDeviceAuthentication: %v", err)
	}
	if issuer.polls.Load() < 2 {
		t.Fatalf("expected an authorization_pending round before success, got %d polls", issuer.polls.Load())
	}

	if _, err := remote.List(ctx); err != nil {
		t.Fatalf("List after device flow: %v", err)
	}
}

// TestServerRemote_NoDeviceFlowAdvertised pins the discovery side: a
// server without OIDC configured must not advertise the device scheme.
func TestServerRemote_NoDeviceFlowAdvertised(t *testing.T) {
	baseURL := startSpikeServer(t, nil)
	r := &ServerRemote{BaseURL: baseURL}
	for _, s := range r.SupportedAuthentication() {
		if s == DeviceAuthenticationCredentialType {
			t.Fatal("device scheme advertised by a server without device flow")
		}
	}
}
