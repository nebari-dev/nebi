// Package netguard restricts an HTTP listener to clients on the local
// machine. Local/desktop mode is a single-user, on-device setup, so the
// listener only accepts requests addressed to a local host and origin.
package netguard

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Middleware wraps next and only allows requests addressed to the local
// machine:
//
//   - The Host header must name a loopback host (localhost, *.localhost,
//     127.0.0.1, ::1). Set allowAnyHost when the operator explicitly bound a
//     non-loopback interface and therefore expects non-loopback Host values.
//
//   - The Origin header, when present, must be a local UI origin (see
//     IsLocalUIOrigin: loopback http(s), the Wails webview, or "null") or
//     one of allowedOrigins (operator-configured via server.allowed_origins /
//     NEBI_SERVER_ALLOWED_ORIGINS, e.g. the public hostname of a reverse
//     proxy such as JupyterHub's jupyter-server-proxy — browsers send the
//     proxy's origin on CORS-mode subresource requests like Vite's
//     crossorigin module bundles). Requests without an Origin (CLI, curl,
//     same-origin GET) pass through.
func Middleware(next http.Handler, allowAnyHost bool, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowAnyHost && !isLoopbackHostPort(r.Host) {
			http.Error(w, "Forbidden: local mode only accepts requests addressed to a local host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !IsLocalUIOrigin(origin) && !OriginAllowed(origin, allowedOrigins) {
			http.Error(w, "Forbidden: local mode only accepts requests from a local origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OriginAllowed reports whether origin matches one of allowed, compared
// case-insensitively and ignoring surrounding whitespace and trailing
// slashes (tolerating hand-typed env var values).
func OriginAllowed(origin string, allowed []string) bool {
	norm := normalizeOrigin(origin)
	if norm == "" {
		return false
	}
	for _, a := range allowed {
		if normalizeOrigin(a) == norm {
			return true
		}
	}
	return false
}

func normalizeOrigin(origin string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))
}

// IsLocalUIOrigin reports whether an Origin header value is one a local UI
// surface produces: a loopback http(s) origin (e.g. the Vite dev server), a
// Wails webview origin ("wails://..." on macOS/Linux), or the opaque "null"
// origin some webviews produce. Used by both this package's Middleware and
// the API's CORS middleware so the two checks cannot drift apart.
func IsLocalUIOrigin(origin string) bool {
	return IsLoopbackOrigin(origin) || strings.HasPrefix(origin, "wails://") || origin == "null"
}

// IsLoopbackOrigin reports whether an Origin header value is an http(s)
// origin on a loopback host (e.g. "http://localhost:8461").
func IsLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

// IsLoopbackHost reports whether a bare hostname or IP (no port) is a
// loopback host. Used to decide whether a configured bind host keeps the
// listener private to this machine.
func IsLoopbackHost(host string) bool {
	return isLoopbackHost(host)
}

// isLoopbackHostPort reports whether a Host header value ("host", "host:port",
// "[v6]:port") names a loopback host.
func isLoopbackHostPort(hostPort string) bool {
	host := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		host = h
	} else {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
