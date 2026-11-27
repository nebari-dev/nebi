// Package ui serves the client UI app (frontend.ClientApp()) for the
// process that owns the client database, with the static-file and SPA
// fallback handler of internal/frontend, and holds the caching and header
// policy for serving it on loopback.
package ui

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/frontend"
)

const (
	scriptNonceKey = "cspScriptNonce"
	styleNonceKey  = "cspStyleNonce"
)

// Options configures the UI handler.
type Options struct {
	// Assets is the built client app, usually frontend.ClientApp().
	// Required.
	Assets fs.FS
	// Guard, when set, makes the handler accept the sign-in URL: it swaps
	// the token in the URL for the session cookie (api.Guard.SignIn). The
	// desktop app's in-process handler has no listener and no guard.
	Guard *api.Guard
	// Logger receives errors reading the assets. Nil means slog.Default().
	Logger *slog.Logger
}

// New returns the handler that serves the client UI. Requests under /api
// are not its business: it answers them with 404.
func New(opts Options) (http.Handler, error) {
	if opts.Assets == nil {
		return nil, errors.New("ui: Options.Assets is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if gin.Mode() == gin.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(gin.Recovery(), headers)
	static := frontend.Handler(opts.Assets, frontend.HandlerOptions{
		ScriptNonceKey: scriptNonceKey,
		StyleNonceKey:  styleNonceKey,
	}, logger)
	engine.NoRoute(static)

	var h http.Handler = engine
	if opts.Guard != nil {
		h = opts.Guard.SignIn(h)
	}
	return h, nil
}

// headers sets the loopback header and caching policy. The UI is served
// from loopback to one user, so it is never framed, sends no referrer, and
// talks to its own origin only. Vite's hashed assets never change and are
// cached for a year; index.html, which the SPA fallback serves for every
// route, is never cached, so a new build is picked up on the next load.
func headers(c *gin.Context) {
	scriptNonce, err := nonce()
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	styleNonce, err := nonce()
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Set(scriptNonceKey, scriptNonce)
	c.Set(styleNonceKey, styleNonce)

	h := c.Writer.Header()
	h.Set("Content-Security-Policy", contentSecurityPolicy(scriptNonce, styleNonce))
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")
	if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-store")
	}
	c.Next()
}

func contentSecurityPolicy(scriptNonce, styleNonce string) string {
	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"script-src 'self' 'nonce-" + scriptNonce + "'",
		"style-src 'self' 'nonce-" + styleNonce + "'",
		"style-src-elem 'self' 'nonce-" + styleNonce + "'",
		"style-src-attr 'none'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"manifest-src 'self'",
		"form-action 'self'",
	}, "; ")
}

func nonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b[:]), nil
}
