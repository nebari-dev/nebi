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
	guard := netguard.Middleware(okHandler(), false, nil)

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
	guard := netguard.Middleware(okHandler(), false, nil)

	req := httptest.NewRequest(http.MethodGet, "http://evil.example.com:8460/api/v1/workspaces", nil)
	rec := httptest.NewRecorder()
	guard.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback Host, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsNonLocalOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false, nil)

	for _, origin := range []string{
		"https://evil.example.com",
		"http://evil.example.com:8460",
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

// The Wails webview reaches the network listener directly in `wails dev`
// (VITE_API_URL points the frontend at it), sending "wails://..." Origins on
// macOS/Linux — or "null" from some webviews. These must pass, matching the
// origins corsMiddleware echoes (issue #530).
func TestMiddlewareAllowsWebviewOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false, nil)

	for _, origin := range []string{
		"wails://wails.localhost",
		"wails://wails.localhost:34115",
		"null",
	} {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:8460/api/v1/workspaces", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Origin %q: expected 200, got %d", origin, rec.Code)
		}
	}
}

func TestMiddlewareAllowAnyHostStillRejectsNonLocalOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), true, nil)

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

func TestMiddlewareAllowsConfiguredOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false, []string{"https://hub.example.com", "https://other.example.com:8443"})

	for _, origin := range []string{
		"https://hub.example.com",
		"HTTPS://HUB.EXAMPLE.COM", // case-insensitive
		"https://other.example.com:8443",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://localhost:8460/assets/index.js", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Origin %q: expected 200, got %d", origin, rec.Code)
		}
	}
}

func TestMiddlewareConfiguredOriginsDoNotOpenOtherOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false, []string{"https://hub.example.com"})

	for _, origin := range []string{
		"https://evil.example.com",
		"https://hub.example.com.evil.com",
		"http://hub.example.com", // scheme must match too
	} {
		req := httptest.NewRequest(http.MethodGet, "http://localhost:8460/assets/index.js", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Origin %q: expected 403, got %d", origin, rec.Code)
		}
	}
}

func TestOriginAllowed(t *testing.T) {
	allowed := []string{" https://hub.example.com/ "} // tolerates stray whitespace/slash from env vars
	if !netguard.OriginAllowed("https://hub.example.com", allowed) {
		t.Error("expected normalized match to be allowed")
	}
	if netguard.OriginAllowed("", allowed) {
		t.Error("empty origin must not match")
	}
	if netguard.OriginAllowed("https://hub.example.com", nil) {
		t.Error("nil allowlist must not match")
	}
}

func TestMiddlewareAllowsLoopbackAndAbsentOrigins(t *testing.T) {
	guard := netguard.Middleware(okHandler(), false, nil)

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
