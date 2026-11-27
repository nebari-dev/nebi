package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const testAddr = "127.0.0.1:8460"

func newTestGuard(t *testing.T) (*Guard, *Session) {
	t.Helper()
	s, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewGuard(s, testAddr)
	if err != nil {
		t.Fatal(err)
	}
	return g, s
}

// protected is the API as a listener serves it: the host check around the
// token check around the router.
func protected(t *testing.T, g *Guard, opts Options) http.Handler {
	t.Helper()
	return g.RequireHost(g.RequireToken(newHandler(t, opts)))
}

func onHost(r *http.Request) { r.Host = testAddr }

func bearer(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func cookie(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: value}) }
}

func header(k, v string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(k, v) }
}

func TestNewSessionToken(t *testing.T) {
	a, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(a.Token())
	if err != nil || len(raw) != 32 {
		t.Fatalf("token %q is not 32 bytes in base64url: %v", a.Token(), err)
	}
	if a.Token() == b.Token() {
		t.Fatal("two sessions got the same token")
	}
	if !a.Valid(a.Token()) || a.Valid(b.Token()) || a.Valid("") || a.Valid(a.Token()+"x") {
		t.Fatal("Valid accepts the wrong tokens")
	}
	for _, verb := range []string{"%v", "%+v", "%s"} {
		if out := fmt.Sprintf(verb, a); strings.Contains(out, a.Token()) {
			t.Errorf("%s printed the token: %s", verb, out)
		}
	}
}

// allRoutes lists every route the API registers, with test values for path
// wildcards, so a route a resource issue adds is covered too.
func allRoutes(t *testing.T, h http.Handler) [][2]string {
	t.Helper()
	r, ok := h.(*router)
	if !ok {
		t.Fatalf("New returned %T, want *router", h)
	}
	wildcard := regexp.MustCompile(`\{[^}]+\}`)
	var routes [][2]string
	for pattern, methods := range r.routes {
		path := wildcard.ReplaceAllString(pattern, "00000000-0000-0000-0000-000000000001")
		for m := range methods {
			routes = append(routes, [2]string{m, path})
		}
	}
	slices.SortFunc(routes, func(a, b [2]string) int { return strings.Compare(a[1]+a[0], b[1]+b[0]) })
	if len(routes) < 4 {
		t.Fatalf("found only %d routes: %v", len(routes), routes)
	}
	return routes
}

// Every /api/v1 route except GET /version refuses a request without the
// token or with a wrong one, and lets the right one through as a bearer
// token or as the session cookie.
func TestTokenRequiredOnEveryRoute(t *testing.T) {
	opts := testOptions(t)
	opts.FolderPicker = fakePicker{path: "/x"}
	opts.Window = &fakeWindow{}
	g, s := newTestGuard(t)
	h := protected(t, g, opts)
	origin := header("Origin", "http://"+testAddr)

	routes := append(allRoutes(t, newHandler(t, opts)), [2]string{http.MethodGet, "/api/v1/no-such-route"})
	for _, rt := range routes {
		method, path := rt[0], rt[1]
		t.Run(method+" "+path, func(t *testing.T) {
			open := method == http.MethodGet && path == "/api/v1/version"
			for name, auth := range map[string][]func(*http.Request){
				"no token":          nil,
				"wrong bearer":      {bearer("wrong")},
				"empty bearer":      {bearer("")},
				"basic scheme":      {header("Authorization", "Basic "+s.Token())},
				"token without":     {header("Authorization", s.Token())},
				"wrong cookie":      {cookie(g.cookie, "wrong"), origin},
				"other port cookie": {cookie(cookiePrefix+"9999", s.Token()), origin},
				"token in query":    {func(r *http.Request) { r.URL.RawQuery = "token=" + s.Token() }},
				// A wrong bearer token is refused even with a good cookie.
				"wrong bearer good cookie": {bearer("wrong"), cookie(g.cookie, s.Token()), origin},
			} {
				rec := do(h, method, path, append([]func(*http.Request){onHost}, auth...)...)
				if open {
					if rec.Code != http.StatusOK {
						t.Errorf("%s: GET /version = %d, want 200 without a token", name, rec.Code)
					}
					continue
				}
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("%s: %d %s, want 401", name, rec.Code, rec.Body)
					continue
				}
				if body := decodeError(t, rec); body.Code != CodeUnauthorized {
					t.Errorf("%s: code %s, want %s", name, body.Code, CodeUnauthorized)
				}
				if rec.Header().Get("WWW-Authenticate") == "" {
					t.Errorf("%s: 401 without WWW-Authenticate", name)
				}
			}
			for name, auth := range map[string][]func(*http.Request){
				"bearer":           {bearer(s.Token())},
				"lowercase bearer": {header("Authorization", "bearer "+s.Token())},
				"cookie":           {cookie(g.cookie, s.Token()), origin},
			} {
				rec := do(h, method, path, append([]func(*http.Request){onHost}, auth...)...)
				if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
					t.Errorf("%s: %d %s, want the request let through", name, rec.Code, rec.Body)
				}
			}
		})
	}
}

// A request that changes state and is authenticated by the cookie must come
// from the listener's own origin: a browser sends the cookie with requests
// from other pages on the same site, such as another port on 127.0.0.1.
func TestCookieStateChangeNeedsOrigin(t *testing.T) {
	opts := testOptions(t)
	opts.Window = &fakeWindow{}
	g, s := newTestGuard(t)
	h := protected(t, g, opts)
	withCookie := cookie(g.cookie, s.Token())
	path := "/api/v1/desktop/show-window"

	for name, origin := range map[string]string{
		"no origin":      "",
		"other port":     "http://127.0.0.1:9999",
		"localhost name": "http://localhost:8460",
		"https":          "https://" + testAddr,
		"null":           "null",
		"other site":     "https://evil.example",
	} {
		edits := []func(*http.Request){onHost, withCookie}
		if origin != "" {
			edits = append(edits, header("Origin", origin))
		}
		rec := do(h, http.MethodPost, path, edits...)
		if rec.Code != http.StatusForbidden || decodeError(t, rec).Code != CodeForbiddenOrigin {
			t.Errorf("%s: POST with the cookie = %d %s, want 403 %s", name, rec.Code, rec.Body, CodeForbiddenOrigin)
		}
	}
	if rec := do(h, http.MethodPost, path, onHost, withCookie, header("Origin", "http://"+testAddr)); rec.Code != http.StatusNoContent {
		t.Errorf("POST with the cookie from the own origin = %d %s", rec.Code, rec.Body)
	}
	// Reads need no Origin; the browser does not let another origin read
	// the response, because no CORS header allows it.
	if rec := do(h, http.MethodGet, "/api/v1/capabilities", onHost, withCookie); rec.Code != http.StatusOK {
		t.Errorf("GET with the cookie and no Origin = %d %s", rec.Code, rec.Body)
	}
	// The CLI sends the bearer token and no Origin.
	if rec := do(h, http.MethodPost, path, onHost, bearer(s.Token())); rec.Code != http.StatusNoContent {
		t.Errorf("POST with the bearer token and no Origin = %d %s", rec.Code, rec.Body)
	}
}

// A request whose Host is not the listener's address is refused, which
// blocks DNS rebinding.
func TestForeignHostRefused(t *testing.T) {
	g, s := newTestGuard(t)
	h := protected(t, g, testOptions(t))
	for _, host := range []string{"evil.example", "evil.example:8460", "localhost:8460", "127.0.0.1", "127.0.0.1:9999", "[::1]:8460", ""} {
		for _, path := range []string{"/api/v1/version", "/api/v1/capabilities"} {
			rec := do(h, http.MethodGet, path, bearer(s.Token()), func(r *http.Request) { r.Host = host })
			if rec.Code != http.StatusForbidden || decodeError(t, rec).Code != CodeForbiddenHost {
				t.Errorf("GET %s with Host %q = %d %s, want 403 %s", path, host, rec.Code, rec.Body, CodeForbiddenHost)
			}
		}
	}
	if rec := do(h, http.MethodGet, "/api/v1/version", onHost); rec.Code != http.StatusOK {
		t.Errorf("GET /version with the right Host = %d", rec.Code)
	}
}

// No response allows another origin to read it.
func TestNoCORSHeaders(t *testing.T) {
	opts := testOptions(t)
	opts.Window = &fakeWindow{}
	g, s := newTestGuard(t)
	h := protected(t, g, opts)
	evil := header("Origin", "https://evil.example")
	for _, rec := range []*httptest.ResponseRecorder{
		do(h, http.MethodGet, "/api/v1/version", onHost, evil),
		do(h, http.MethodGet, "/api/v1/capabilities", onHost, evil, bearer(s.Token())),
		do(h, http.MethodOptions, "/api/v1/capabilities", onHost, evil, header("Access-Control-Request-Method", "POST")),
		do(h, http.MethodPost, "/api/v1/desktop/show-window", onHost, evil, cookie(g.cookie, s.Token())),
		do(h, http.MethodGet, "/api/v1/version", evil),
	} {
		for k := range rec.Header() {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Errorf("response %d carries %s: %s", rec.Code, k, rec.Header().Get(k))
			}
		}
	}
}

func TestSignIn(t *testing.T) {
	g, s := newTestGuard(t)
	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ui")) })
	h := g.SignIn(ui)

	if g.SignInURL() != "http://"+testAddr+"/?token="+s.Token() {
		t.Fatalf("SignInURL = %q", g.SignInURL())
	}

	for target, want := range map[string]string{
		"/?token=" + s.Token():                        "/",
		"/projects/1?tab=versions&token=" + s.Token(): "/projects/1?tab=versions",
	} {
		rec := do(h, http.MethodGet, target)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Fatalf("GET %s = %d to %q, want 303 to %q", target, rec.Code, rec.Header().Get("Location"), want)
		}
		if strings.Contains(rec.Header().Get("Location"), s.Token()) {
			t.Fatal("the redirect keeps the token in the address")
		}
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("GET %s set %d cookies, want 1", target, len(cookies))
		}
		c := cookies[0]
		if c.Name != g.cookie || c.Value != s.Token() || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode ||
			c.Path != "/" || c.MaxAge != 0 || !c.Expires.IsZero() {
			t.Fatalf("cookie = %+v, want an HttpOnly, SameSite=Strict session cookie for /", c)
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("sign-in response headers = %v", rec.Header())
		}
	}

	// A wrong token sets no cookie and does not reach the UI.
	rec := do(h, http.MethodGet, "/?token=wrong")
	if rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 || rec.Body.String() == "ui" {
		t.Fatalf("GET with a wrong token = %d %q, cookies %v", rec.Code, rec.Body, rec.Result().Cookies())
	}

	// The redirect never leaves the origin.
	rec = do(h, http.MethodGet, "//evil.example/?token="+s.Token())
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("redirect for //evil.example = %q, want /", loc)
	}

	// Without a token, and for other methods, the UI is served.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		target := "/"
		if method == http.MethodPost {
			target = "/?token=" + s.Token()
		}
		if rec := do(h, method, target); rec.Body.String() != "ui" {
			t.Errorf("%s %s = %d %q, want the UI", method, target, rec.Code, rec.Body)
		}
	}
}

func TestNewGuardRequiresLoopback(t *testing.T) {
	s, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"0.0.0.0:8460", "192.168.1.2:8460", "localhost:8460", "127.0.0.1", ""} {
		if _, err := NewGuard(s, addr); err == nil {
			t.Errorf("NewGuard(%q) succeeded", addr)
		}
	}
	g, err := NewGuard(s, "127.0.0.1:43210")
	if err != nil {
		t.Fatal(err)
	}
	if g.cookie != "nebi_session_43210" {
		t.Errorf("cookie name = %q, want it to name the port", g.cookie)
	}
}
