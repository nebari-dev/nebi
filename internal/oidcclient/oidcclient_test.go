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

// TestWaitForDeviceTokenHonoursSlowDown pins the RFC 8628 polling rules:
// slow_down adds five seconds to the interval, authorization_pending keeps
// polling, and the first success is returned.
func TestWaitForDeviceTokenHonoursSlowDown(t *testing.T) {
	var polls atomic.Int32
	var pollTimes []time.Time
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pollTimes = append(pollTimes, time.Now())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch polls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"slow_down"}`))
		default:
			_, _ = w.Write([]byte(`{"access_token":"ok","token_type":"Bearer","expires_in":60}`))
		}
	}))
	defer srv.Close()

	// Use a sub-second interval by expressing it through a zero Interval
	// (0s) so the test stays fast; slow_down then raises it to 5s, which we
	// observe as the gap before the third poll. Keep the test bounded.
	da := &DeviceAuthorization{DeviceCode: "dc", Interval: 0, ExpiresIn: 30}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	tok, err := WaitForDeviceToken(ctx, &Endpoints{Token: srv.URL}, "cli", da)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "ok" {
		t.Fatalf("got %q", tok.AccessToken)
	}
	if polls.Load() != 3 {
		t.Fatalf("polled %d times, want 3", polls.Load())
	}
	if elapsed := time.Since(start); elapsed < 5*time.Second {
		t.Fatalf("slow_down was not applied: finished in %s", elapsed)
	}
	mu.Lock()
	gap := pollTimes[2].Sub(pollTimes[1])
	mu.Unlock()
	if gap < 5*time.Second {
		t.Fatalf("gap after slow_down was %s, want >= 5s", gap)
	}
}

func TestWaitForDeviceTokenReturnsExpiredAndDenied(t *testing.T) {
	for _, tc := range []struct {
		code string
		want error
	}{{"expired_token", ErrExpired}, {"access_denied", ErrAccessDenied}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"` + tc.code + `"}`))
		}))
		da := &DeviceAuthorization{DeviceCode: "dc", Interval: 0, ExpiresIn: 30}
		_, err := WaitForDeviceToken(context.Background(), &Endpoints{Token: srv.URL}, "cli", da)
		srv.Close()
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.code, err, tc.want)
		}
	}
}

func TestWaitForDeviceTokenStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	da := &DeviceAuthorization{DeviceCode: "dc", Interval: 0, ExpiresIn: 30}
	_, err := WaitForDeviceToken(ctx, &Endpoints{Token: srv.URL}, "cli", da)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context deadline", err)
	}
}
