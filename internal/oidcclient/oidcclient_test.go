package oidcclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/auth/authtest"
	"golang.org/x/oauth2"
)

// TestTokenSourceConcurrentRefreshNotifiesOnce pins that callers sharing one
// refreshing source (the desktop app serves parallel requests from it) do
// not race on the cached token and persist a refreshed token exactly once.
// Run with -race.
func TestTokenSourceConcurrentRefreshNotifiesOnce(t *testing.T) {
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new","token_type":"Bearer","expires_in":3600,"refresh_token":"r2"}`))
	}))
	defer srv.Close()

	expired := &oauth2.Token{AccessToken: "old", RefreshToken: "r1", Expiry: time.Now().Add(-time.Hour)}
	var saved atomic.Int32
	src := TokenSource(context.Background(), srv.URL, "cli", expired, func(tok *oauth2.Token) error {
		saved.Add(1)
		if tok.RefreshToken != "r2" {
			t.Errorf("onRefresh got refresh token %q, want r2", tok.RefreshToken)
		}
		return nil
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := src.Token()
			if err != nil {
				t.Error(err)
				return
			}
			if tok.AccessToken != "new" {
				t.Errorf("got access token %q, want new", tok.AccessToken)
			}
		}()
	}
	wg.Wait()

	if got := tokenCalls.Load(); got != 1 {
		t.Errorf("token endpoint called %d times, want 1", got)
	}
	if got := saved.Load(); got != 1 {
		t.Errorf("onRefresh called %d times, want 1", got)
	}
}

func TestTokenSourceWithoutRefreshTokenIsStatic(t *testing.T) {
	tok := &oauth2.Token{AccessToken: "fixed"}
	src := TokenSource(context.Background(), "http://unused", "cli", tok, func(*oauth2.Token) error {
		t.Fatal("onRefresh must not be called")
		return nil
	})
	got, err := src.Token()
	if err != nil || got.AccessToken != "fixed" {
		t.Fatalf("got %v, %v", got, err)
	}
}

// TestDeviceFlowAgainstProvider runs discovery, the device authorization and
// the token wait against an in-process provider that enforces PKCE, so the
// verifier must reach the token request.
func TestDeviceFlowAgainstProvider(t *testing.T) {
	idp, err := authtest.NewServer("cli")
	if err != nil {
		t.Fatal(err)
	}
	defer idp.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg, err := Discover(ctx, idp.URL, "cli", []string{"openid", OfflineAccessScope})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Endpoint.TokenURL != idp.URL+"/token" || cfg.Endpoint.DeviceAuthURL != idp.URL+"/device" {
		t.Fatalf("unexpected endpoint %+v", cfg.Endpoint)
	}
	da, err := StartDeviceAuthorization(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if da.Verifier == "" || da.BrowseURL() == "" {
		t.Fatalf("unexpected device authorization %+v", da)
	}
	if err := idp.ApproveDevice(da.UserCode, authtest.Identity{Subject: "sub-1", Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	tok, err := WaitForDeviceToken(ctx, cfg, da)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Fatalf("unexpected token %+v", tok)
	}
}

func TestWaitForDeviceTokenAccessDenied(t *testing.T) {
	idp, err := authtest.NewServer("cli")
	if err != nil {
		t.Fatal(err)
	}
	defer idp.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg, err := Discover(ctx, idp.URL, "cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	da, err := StartDeviceAuthorization(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := idp.DenyDevice(da.UserCode); err != nil {
		t.Fatal(err)
	}
	if _, err := WaitForDeviceToken(ctx, cfg, da); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("got %v, want ErrAccessDenied", err)
	}
}

// tokenEndpoint serves a fixed RFC 8628 error from a token endpoint.
func tokenEndpoint(t *testing.T, code string) *oauth2.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"` + code + `"}`))
	}))
	t.Cleanup(srv.Close)
	return &oauth2.Config{ClientID: "cli", Endpoint: oauth2.Endpoint{TokenURL: srv.URL, AuthStyle: oauth2.AuthStyleInParams}}
}

func pendingDevice(expiresIn time.Duration) *DeviceAuthorization {
	return &DeviceAuthorization{
		DeviceAuthResponse: oauth2.DeviceAuthResponse{DeviceCode: "dc", Interval: 1, Expiry: time.Now().Add(expiresIn)},
		Verifier:           "verifier",
	}
}

func TestWaitForDeviceTokenExpiredTokenError(t *testing.T) {
	_, err := WaitForDeviceToken(context.Background(), tokenEndpoint(t, "expired_token"), pendingDevice(time.Minute))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("got %v, want ErrExpired", err)
	}
}

// TestWaitForDeviceTokenStopsAtExpiry pins that polling ends at the device
// code's expiry even while the provider keeps answering authorization_pending.
func TestWaitForDeviceTokenStopsAtExpiry(t *testing.T) {
	_, err := WaitForDeviceToken(context.Background(), tokenEndpoint(t, "authorization_pending"), pendingDevice(1500*time.Millisecond))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("got %v, want ErrExpired", err)
	}
}

func TestWaitForDeviceTokenStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := WaitForDeviceToken(ctx, tokenEndpoint(t, "authorization_pending"), pendingDevice(time.Minute))
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrExpired) {
		t.Fatalf("got %v, want the caller's context deadline", err)
	}
}
