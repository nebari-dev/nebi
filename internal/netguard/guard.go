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
//   - The Origin header, when present, must be a loopback http(s) origin.
//     Requests without an Origin (CLI, curl, same-origin GET) pass through.
func Middleware(next http.Handler, allowAnyHost bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowAnyHost && !isLoopbackHostPort(r.Host) {
			http.Error(w, "Forbidden: local mode only accepts requests addressed to a local host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !IsLoopbackOrigin(origin) {
			http.Error(w, "Forbidden: local mode only accepts requests from a local origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
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
