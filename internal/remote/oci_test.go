package remote

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

// startAuthedRegistry runs an in-memory OCI Distribution server that
// requires HTTP basic auth (testuser/testpass) on every request, the
// closest local stand-in for a private registry.
func startAuthedRegistry(t *testing.T) string {
	t.Helper()
	inner := registry.New()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != want {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return u.Host
}

// TestOCIRemote_Conformance runs the shared lifecycle script against a
// private (basic-auth) OCI registry.
func TestOCIRemote_Conformance(t *testing.T) {
	host := startAuthedRegistry(t)
	r := &OCIRemote{Host: host, Namespace: "team", PlainHTTP: true}

	good := AuthenticationCredential{
		Type:     BasicAuthenticationCredentialType,
		Username: "testuser",
		Password: "testpass",
	}
	bad := AuthenticationCredential{
		Type:     BasicAuthenticationCredentialType,
		Username: "testuser",
		Password: "wrong",
	}
	runRemoteConformance(t, r, good, bad, "no-such-repo")
}

// TestOCIRemote_Anonymous covers the public read-only registry case
// from the issue: SupportedAuthentication is empty and the client uses
// the remote without ever calling Authenticate.
func TestOCIRemote_Anonymous(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	r := &OCIRemote{Host: u.Host, PlainHTTP: true, AllowAnonymous: true}
	if got := r.SupportedAuthentication(); len(got) != 0 {
		t.Fatalf("SupportedAuthentication: want empty, got %v", got)
	}

	ctx := context.Background()
	err = r.Push(ctx, Workspace{
		WorkspacePreview: WorkspacePreview{Name: "pub-demo"},
		Ref:              "v1",
		PixiToml:         conformanceTOML,
		PixiLock:         conformanceLock,
	})
	if err != nil {
		t.Fatalf("Push without auth: %v", err)
	}
	ws, err := r.Pull(ctx, "pub-demo")
	if err != nil {
		t.Fatalf("Pull without auth: %v", err)
	}
	if ws.PixiToml == "" {
		t.Fatal("Pull without auth: empty pixi.toml")
	}
	if _, err := r.Pull(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Pull(missing): want ErrNotFound, got %v", err)
	}
}
