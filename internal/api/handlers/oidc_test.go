package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newTestContext builds a gin.Context wrapping the given request and a fresh
// recorder, for exercising handler-level helpers directly.
func newTestContext(req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c, rec
}

func TestIsRequestHTTPS_PlainHTTP(t *testing.T) {
	c, _ := newTestContext(httptest.NewRequest(http.MethodGet, "http://nebi.example.com/auth/oidc/login", nil))
	if isRequestHTTPS(c) {
		t.Fatal("plain HTTP request must not be treated as HTTPS")
	}
}

func TestIsRequestHTTPS_DirectTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://nebi.example.com/auth/oidc/login", nil)
	req.TLS = &tls.ConnectionState{}
	c, _ := newTestContext(req)
	if !isRequestHTTPS(c) {
		t.Fatal("request served over TLS must be treated as HTTPS")
	}
}

func TestIsRequestHTTPS_ForwardedProto(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nebi.example.com/auth/oidc/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	c, _ := newTestContext(req)
	if !isRequestHTTPS(c) {
		t.Fatal("X-Forwarded-Proto: https (TLS-terminating proxy) must be treated as HTTPS")
	}
}

func TestSetStateCookie_FlagsOverHTTP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nebi.example.com/auth/oidc/login", nil)
	c, rec := newTestContext(req)

	setStateCookie(c, "state-value", 600)

	sc := rec.Header().Get("Set-Cookie")
	if !strings.Contains(sc, "oidc_state=state-value") {
		t.Fatalf("expected oidc_state cookie, got %q", sc)
	}
	if !strings.Contains(sc, "SameSite=Lax") {
		t.Fatalf("expected SameSite=Lax, got %q", sc)
	}
	if !strings.Contains(sc, "HttpOnly") {
		t.Fatalf("expected HttpOnly, got %q", sc)
	}
	if strings.Contains(sc, "Secure") {
		t.Fatalf("plain HTTP must not set Secure, got %q", sc)
	}
}

func TestSetStateCookie_SecureOverHTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nebi.example.com/auth/oidc/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	c, rec := newTestContext(req)

	setStateCookie(c, "state-value", 600)

	sc := rec.Header().Get("Set-Cookie")
	if !strings.Contains(sc, "Secure") {
		t.Fatalf("HTTPS request must set Secure, got %q", sc)
	}
	if !strings.Contains(sc, "SameSite=Lax") {
		t.Fatalf("expected SameSite=Lax, got %q", sc)
	}
}
