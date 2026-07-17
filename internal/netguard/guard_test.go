package netguard_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nebari-dev/nebi/internal/netguard"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddlewareAllowsLoopbackHosts(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false)

	for _, host := range []string{
		"localhost:8460",
		"localhost",
		"127.0.0.1:8460",
		"[::1]:8460",
		"wails.localhost",
		"LOCALHOST:8460",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://placeholder/api/v1/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Host %q: expected 200, got %d", host, rec.Code)
		}
	}
}

func TestMiddlewareRejectsNonLoopbackHost(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false)

	req := httptest.NewRequest(http.MethodGet, "http://evil.example.com:8460/api/v1/workspaces", nil)
	rec := httptest.NewRecorder()
	guard.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback Host, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsNonLocalOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false)

	for _, origin := range []string{
		"https://evil.example.com",
		"http://evil.example.com:8460",
		"null",
		"file://",
	} {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:8460/api/v1/workspaces", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Origin %q: expected 403, got %d", origin, rec.Code)
		}
	}
}

func TestMiddlewareAllowAnyHostStillRejectsNonLocalOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), true)

	req := httptest.NewRequest(http.MethodGet, "http://192.0.2.10:8460/api/v1/health", nil)
	rec := httptest.NewRecorder()
	guard.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allowAnyHost: expected 200 for non-loopback Host, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "http://192.0.2.10:8460/api/v1/workspaces", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	guard.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("allowAnyHost: expected 403 for non-local Origin, got %d", rec.Code)
	}
}

func TestMiddlewareAllowsLoopbackAndAbsentOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false)

	for _, origin := range []string{
		"", // CLI / same-origin GET: no Origin header
		"http://localhost:8460",
		"http://localhost:8461", // vite dev server
		"http://127.0.0.1:8460",
		"http://[::1]:8460",
		"https://localhost:8460",
		"http://wails.localhost",
	} {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:8460/api/v1/workspaces", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Origin %q: expected 200, got %d", origin, rec.Code)
		}
	}
}
