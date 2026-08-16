package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func securityHeadersTestRouter(localMode bool, allowedOrigins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityHeadersMiddleware(localMode, allowedOrigins))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

func doSecurityHeadersRequest(t *testing.T, localMode bool, setup func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	if setup != nil {
		setup(req)
	}
	rec := httptest.NewRecorder()
	securityHeadersTestRouter(localMode, nil).ServeHTTP(rec, req)
	return rec
}

func TestSecurityHeadersMiddlewareSetsBrowserHardeningHeaders(t *testing.T) {
	rec := doSecurityHeadersRequest(t, false, nil)
	headers := rec.Header()

	csp := headers.Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-ancestors 'self'",
		"style-src 'self'",
		"style-src-attr 'none'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"manifest-src 'self'",
		"form-action 'self'",
	} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("expected CSP to include %q, got %q", directive, csp)
		}
	}
	if !regexp.MustCompile(`script-src 'self' 'nonce-[A-Za-z0-9+/]{22}'`).MatchString(csp) {
		t.Fatalf("expected CSP script-src nonce, got %q", csp)
	}
	if !regexp.MustCompile(`style-src-elem 'self' 'nonce-[A-Za-z0-9+/]{22}'`).MatchString(csp) {
		t.Fatalf("expected CSP style-src-elem nonce, got %q", csp)
	}
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("CSP must not allow unsafe-inline, got %q", csp)
	}
	if strings.Contains(csp, "http:") || strings.Contains(csp, "https:") {
		t.Fatalf("team-mode CSP must not allow arbitrary or loopback external origins, got %q", csp)
	}

	if got := headers.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("expected Referrer-Policy, got %q", got)
	}
	if got := headers.Get("Permissions-Policy"); !strings.Contains(got, "camera=()") || !strings.Contains(got, "microphone=()") {
		t.Fatalf("expected restrictive Permissions-Policy, got %q", got)
	}
	if got := headers.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %q", got)
	}
	if got := headers.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("expected X-Frame-Options SAMEORIGIN, got %q", got)
	}
	if got := headers.Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("plain HTTP response must not set HSTS, got %q", got)
	}
}

func TestSecurityHeadersMiddlewareAllowsConfiguredFrameAncestors(t *testing.T) {
	origins := []string{"https://hub.example.com", "https://other.example.com"}
	for _, localMode := range []bool{true, false} {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rec := httptest.NewRecorder()
		securityHeadersTestRouter(localMode, origins).ServeHTTP(rec, req)

		csp := rec.Header().Get("Content-Security-Policy")
		want := "frame-ancestors 'self' https://hub.example.com https://other.example.com"
		if !strings.Contains(csp, want) {
			t.Fatalf("localMode=%v: expected CSP to include %q, got %q", localMode, want, csp)
		}
	}
}

func TestSecurityHeadersMiddlewareAllowsLoopbackConnectOnlyInLocalMode(t *testing.T) {
	rec := doSecurityHeadersRequest(t, true, nil)
	csp := rec.Header().Get("Content-Security-Policy")

	for _, directive := range []string{
		"connect-src 'self' http://localhost:* http://127.0.0.1:*",
		"https://localhost:*",
		"https://127.0.0.1:*",
	} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("expected local-mode CSP to include %q, got %q", directive, csp)
		}
	}
	if strings.Contains(csp, "ws://") {
		t.Fatalf("local-mode CSP should not allow websocket origins that the app does not use, got %q", csp)
	}
}

func TestSecurityHeadersMiddlewareSetsHSTSForHTTPS(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*http.Request)
	}{
		{
			name: "direct TLS",
			setup: func(req *http.Request) {
				req.TLS = &tls.ConnectionState{}
			},
		},
		{
			name: "forwarded proto",
			setup: func(req *http.Request) {
				req.Header.Set("X-Forwarded-Proto", "https")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doSecurityHeadersRequest(t, false, tc.setup)
			if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
				t.Fatalf("expected HSTS on HTTPS request, got %q", got)
			}
		})
	}
}

func TestSecurityHeadersApplyToPreflightResponses(t *testing.T) {
	r := buildTestRouter(t, "")
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("expected preflight response to include Content-Security-Policy")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected preflight response to include X-Content-Type-Options nosniff, got %q", got)
	}
}

func TestBasePathInjectionUsesCSPNonce(t *testing.T) {
	r := buildTestRouter(t, "/nebi")
	req := httptest.NewRequest(http.MethodGet, "/nebi/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected index.html response, got %d", rec.Code)
	}

	csp := rec.Header().Get("Content-Security-Policy")
	scriptMatches := regexp.MustCompile(`script-src 'self' 'nonce-([^']+)'`).FindStringSubmatch(csp)
	if len(scriptMatches) != 2 {
		t.Fatalf("expected CSP script nonce, got %q", csp)
	}
	styleMatches := regexp.MustCompile(`style-src-elem 'self' 'nonce-([^']+)'`).FindStringSubmatch(csp)
	if len(styleMatches) != 2 {
		t.Fatalf("expected CSP style nonce, got %q", csp)
	}

	want := `<script nonce="` + scriptMatches[1] + `">window.__NEBI_BASE_PATH__="/nebi";</script>`
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("expected base-path script to use CSP nonce %q", scriptMatches[1])
	}

	meta := `<meta name="csp-style-nonce" content="` + styleMatches[1] + `" />`
	if !strings.Contains(rec.Body.String(), meta) {
		t.Fatalf("expected index.html to expose CSP style nonce %q to runtime code", styleMatches[1])
	}
}
