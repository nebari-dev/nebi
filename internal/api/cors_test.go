package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func corsTestRouter(localMode bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware(localMode))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

func doCORSRequest(t *testing.T, r *gin.Engine, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestCORSLocalModeNeverWildcards(t *testing.T) {
	r := corsTestRouter(true)

	for _, origin := range []string{"", "https://evil.example.com", "http://localhost:8461"} {
		rec := doCORSRequest(t, r, origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
			t.Errorf("Origin %q: local mode must not send wildcard Access-Control-Allow-Origin", origin)
		}
	}
}

func TestCORSLocalModeEchoesOnlyLocalOrigins(t *testing.T) {
	r := corsTestRouter(true)

	allowed := []string{
		"http://localhost:8461",
		"http://127.0.0.1:8460",
		"wails://wails",
		"null",
	}
	for _, origin := range allowed {
		rec := doCORSRequest(t, r, origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("Origin %q: expected echo, got %q", origin, got)
		}
		if vary := rec.Header().Get("Vary"); vary != "Origin" {
			t.Errorf("Origin %q: expected Vary: Origin, got %q", origin, vary)
		}
	}

	for _, origin := range []string{"https://evil.example.com", "http://evil.example.com:8460", ""} {
		rec := doCORSRequest(t, r, origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Origin %q: expected no Access-Control-Allow-Origin, got %q", origin, got)
		}
	}
}

func TestCORSTeamModeKeepsWildcard(t *testing.T) {
	r := corsTestRouter(false)

	rec := doCORSRequest(t, r, "https://anywhere.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("team mode: expected wildcard Access-Control-Allow-Origin, got %q", got)
	}
}
