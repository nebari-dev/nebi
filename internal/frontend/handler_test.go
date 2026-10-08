package frontend

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestHandler(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":         {Data: []byte(`<html><head></head><body><script src="/assets/app.js"></script><link href="/assets/app.css"></body></html>`)},
		"assets/app.js":      {Data: []byte(`console.log("app")`)},
		"assets/app.css":     {Data: []byte(`@font-face{src:url(/assets/font.woff2)}`)},
		"public/config.json": {Data: []byte(`{"name":"embedded"}`)},
	}
	brandingPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(brandingPath, []byte(`{"name":"override"}`), 0600); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("script", "script-nonce"); c.Set("style", "style-nonce") })
	router.NoRoute(Handler(assets, HandlerOptions{
		BasePath: "/nebi", BrandingConfigPath: brandingPath,
		ScriptNonceKey: "script", StyleNonceKey: "style",
	}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	for _, tc := range []struct {
		path        string
		status      int
		contentType string
		contains    []string
	}{
		{"/nebi/projects/123", 200, "text/html", []string{`src="/nebi/assets/app.js"`, `href="/nebi/assets/app.css"`, `nonce="script-nonce"`, `window.__NEBI_BASE_PATH__="/nebi"`, `name="csp-style-nonce" content="style-nonce"`}},
		{"/nebi/assets/app.js", 200, "application/javascript", []string{`console.log("app")`}},
		{"/nebi/assets/app.css", 200, "text/css", []string{`url(/nebi/assets/font.woff2)`}},
		{"/nebi/public/config.json", 200, "application/json", []string{`"override"`}},
		{"/nebi/api/missing", 404, "application/json", []string{`Not found`}},
		{"/nebi/docs/missing", 404, "application/json", []string{`Not found`}},
		{"/outside", 404, "application/json", []string{`Not found`}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if !strings.HasPrefix(response.Header().Get("Content-Type"), tc.contentType) {
				t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
			}
			for _, want := range tc.contains {
				if !strings.Contains(response.Body.String(), want) {
					t.Errorf("body does not contain %q: %s", want, response.Body.String())
				}
			}
		})
	}
	if err := os.Remove(brandingPath); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/nebi/public/config.json", nil))
	if response.Body.String() != `{"name":"embedded"}` {
		t.Fatalf("missing override should use embedded config: %s", response.Body.String())
	}
}
