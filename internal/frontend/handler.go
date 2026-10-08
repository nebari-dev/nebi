package frontend

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// HandlerOptions supplies serving policy chosen by the API composition root.
type HandlerOptions struct {
	BasePath           string
	BrandingConfigPath string
	ScriptNonceKey     string
	StyleNonceKey      string
}

// Handler serves static assets with SPA fallback, preserving proxy base paths
// and the runtime branding and CSP metadata supplied by the caller.
func Handler(embedFS fs.FS, options HandlerOptions, logger *slog.Logger) gin.HandlerFunc {
	basePath := options.BasePath
	runtimeBrandingConfigPath := options.BrandingConfigPath
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Strip base path prefix to get the relative path
		relPath := path
		if basePath != "" {
			if !strings.HasPrefix(path, basePath) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				return
			}
			relPath = strings.TrimPrefix(path, basePath)
			if relPath == "" {
				relPath = "/"
			}
		}

		// Runtime branding config override:
		// if a Helm-mounted file exists on disk, serve it instead of embedded assets.
		if relPath == "/public/config.json" {
			content, err := os.ReadFile(runtimeBrandingConfigPath)
			if err == nil {
				c.Data(http.StatusOK, "application/json", content)
				return
			}
			if !os.IsNotExist(err) {
				logger.Warn("Failed to read runtime branding config", "path", runtimeBrandingConfigPath, "error", err)
			}
		}

		// Don't serve HTML for API calls or docs
		if strings.HasPrefix(relPath, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}
		if strings.HasPrefix(relPath, "/docs") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}

		// Remove leading slash for embedded FS
		fsPath := strings.TrimPrefix(relPath, "/")
		if fsPath == "" {
			fsPath = "index.html"
		}

		// Try to open the file in the embedded FS
		file, err := embedFS.Open(fsPath)
		if err != nil {
			// File doesn't exist, serve index.html for SPA routing
			fsPath = "index.html"
			file, err = embedFS.Open(fsPath)
			if err != nil {
				c.String(http.StatusInternalServerError, "Error loading frontend")
				return
			}
		}
		defer file.Close()

		// Read file content
		content, err := io.ReadAll(file)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error reading file")
			return
		}

		// Set content type based on file extension
		contentType := "text/plain"
		if strings.HasSuffix(fsPath, ".html") {
			contentType = "text/html; charset=utf-8"
		} else if strings.HasSuffix(fsPath, ".js") {
			contentType = "application/javascript"
		} else if strings.HasSuffix(fsPath, ".css") {
			contentType = "text/css"
		} else if strings.HasSuffix(fsPath, ".json") {
			contentType = "application/json"
		} else if strings.HasSuffix(fsPath, ".svg") {
			contentType = "image/svg+xml"
		} else if strings.HasSuffix(fsPath, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(fsPath, ".jpg") || strings.HasSuffix(fsPath, ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(fsPath, ".woff2") {
			contentType = "font/woff2"
		} else if strings.HasSuffix(fsPath, ".woff") {
			contentType = "font/woff"
		} else if strings.HasSuffix(fsPath, ".ttf") {
			contentType = "font/ttf"
		}

		// Rewrite absolute url(/...) references in bundled CSS (e.g. self-hosted
		// fonts) to include the base path. Without this they resolve against the
		// domain root and 404 when Nebi is served under a path prefix (proxy).
		if strings.HasSuffix(fsPath, ".css") && basePath != "" {
			content = []byte(strings.ReplaceAll(string(content), `url(/`, `url(`+basePath+`/`))
		}

		// For index.html, inject CSP nonce metadata and rewrite asset URLs
		if fsPath == "index.html" {
			html := string(content)
			nonceAttr := ""
			if styleNonce := c.GetString(options.StyleNonceKey); styleNonce != "" {
				nonceMeta := fmt.Sprintf(`<meta name="csp-style-nonce" content="%s" />`, styleNonce)
				html = strings.Replace(html, "<head>", "<head>\n    "+nonceMeta, 1)
			}
			if basePath != "" {
				if scriptNonce := c.GetString(options.ScriptNonceKey); scriptNonce != "" {
					nonceAttr = fmt.Sprintf(` nonce="%s"`, scriptNonce)
				}
				// Inject base path script tag into <head>
				injection := fmt.Sprintf(`<script%s>window.__NEBI_BASE_PATH__=%q;</script>`, nonceAttr, basePath)
				html = strings.Replace(html, "<head>", "<head>\n    "+injection, 1)
				// Rewrite absolute asset paths to include base path
				html = strings.ReplaceAll(html, `href="/`, `href="`+basePath+`/`)
				html = strings.ReplaceAll(html, `src="/`, `src="`+basePath+`/`)
			}
			content = []byte(html)
		}

		c.Data(http.StatusOK, contentType, content)
	}
}
