package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nebari-dev/nebi/internal/client/api"
)

var testAssets = fstest.MapFS{
	"index.html":          {Data: []byte("<!doctype html><html><head><title>Nebi</title></head><body></body></html>")},
	"assets/index-abc.js": {Data: []byte("console.log(1)")},
	"favicon.svg":         {Data: []byte("<svg/>")},
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestServesAppWithLoopbackPolicy(t *testing.T) {
	h, err := New(Options{Assets: testAssets})
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"/", "/projects/123"} { // SPA fallback
		rec := get(t, h, target)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>Nebi</title>") {
			t.Fatalf("GET %s = %d %q, want index.html", target, rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", target, got)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		for _, d := range []string{"default-src 'self'", "frame-ancestors 'none'", "connect-src 'self'", "script-src 'self' 'nonce-"} {
			if !strings.Contains(csp, d) {
				t.Errorf("GET %s CSP %q lacks %q", target, csp, d)
			}
		}
		// The style nonce reaches the page for the UI's runtime styles.
		if !strings.Contains(rec.Body.String(), `<meta name="csp-style-nonce"`) {
			t.Errorf("GET %s did not inject the style nonce: %s", target, rec.Body)
		}
		for k, v := range map[string]string{
			"Referrer-Policy":        "no-referrer",
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
		} {
			if rec.Header().Get(k) != v {
				t.Errorf("GET %s %s = %q, want %q", target, k, rec.Header().Get(k), v)
			}
		}
		for k := range rec.Header() {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Errorf("GET %s sends %s", target, k)
			}
		}
	}

	rec := get(t, h, "/assets/index-abc.js")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset = %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("hashed asset Content-Type = %q", ct)
	}

	// /api is the client API's, never the UI's.
	if rec := get(t, h, "/api/v1/whatever"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/whatever from the UI = %d, want 404", rec.Code)
	}
}

// With a guard, the UI handler swaps the sign-in URL's token for the
// session cookie.
func TestSignInThroughUI(t *testing.T) {
	session := api.NewSession()
	guard, err := api.NewGuard(session, "127.0.0.1:8460")
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(Options{Assets: testAssets, Guard: guard})
	if err != nil {
		t.Fatal(err)
	}
	rec := get(t, h, "/?token="+session.Token())
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("sign-in URL = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "nebi_session_8460" || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("sign-in cookies = %+v", cookies)
	}
	if rec := get(t, h, "/"); rec.Code != http.StatusOK {
		t.Fatalf("GET / after sign-in = %d", rec.Code)
	}

	// Without a guard (the desktop window, in-process) a token in the URL
	// is ignored.
	plain, err := New(Options{Assets: testAssets})
	if err != nil {
		t.Fatal(err)
	}
	if rec := get(t, plain, "/?token="+session.Token()); rec.Code != http.StatusOK || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("in-process UI with a token = %d, cookies %v", rec.Code, rec.Result().Cookies())
	}
}

func TestNewRequiresAssets(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("New without assets succeeded")
	}
}
