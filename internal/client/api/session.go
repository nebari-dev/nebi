package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// tokenBytes is how many random bytes a session token has.
const tokenBytes = 32

// cookiePrefix starts the name of the session cookie. Cookies are scoped to
// a host, not a port, so the name ends with the port: two owners on one
// machine, each with its own data directory, must not overwrite each
// other's cookie.
const cookiePrefix = "nebi_session_"

// Session is the session token of an owner process. It is a process fence,
// not an identity: it keeps other local users and web pages off the API,
// which listens on loopback. The owner generates it when it starts and
// publishes it only in owner.json and, for nebi-web, in the sign-in URL it
// prints. It is never read from a flag and never logged.
type Session struct {
	token string
}

// NewSession generates a session token of 32 random bytes.
func NewSession() *Session {
	b := make([]byte, tokenBytes)
	// crypto/rand.Read never returns an error: it ends the program
	// instead.
	_, _ = rand.Read(b)
	return &Session{token: base64.RawURLEncoding.EncodeToString(b)}
}

// Token returns the token, to publish in owner.json or the sign-in URL.
func (s *Session) Token() string { return s.token }

// Valid reports whether candidate is the token. The comparison takes the
// same time wherever the two differ.
func (s *Session) Valid(candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(s.token)) == 1
}

// String hides the token from fmt.
func (s *Session) String() string { return "api.Session{token: REDACTED}" }

// Guard protects the client API and the client UI on the loopback listener
// at Addr. The desktop app's own window reaches both in-process, without a
// listener, and needs no guard.
type Guard struct {
	session *Session
	addr    string
	origin  string
	cookie  string
}

// NewGuard returns the guard for a listener bound to addr, the
// "127.0.0.1:port" that net.Listener.Addr reports.
func NewGuard(session *Session, addr string) (*Guard, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("guard address %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("guard address %q is not a loopback IP address", addr)
	}
	return &Guard{session: session, addr: addr, origin: "http://" + addr, cookie: cookiePrefix + port}, nil
}

// SignInURL is the one-time sign-in URL nebi-web prints, as Jupyter does:
// opening it in a browser swaps the token for the session cookie
// (see SignIn).
func (g *Guard) SignInURL() string {
	return g.origin + "/?token=" + g.session.token
}

// RequireHost refuses a request whose Host header is not the address the
// listener is bound to. A web page that rebinds its own DNS name to
// 127.0.0.1 sends its own name as Host, so this blocks DNS rebinding.
func (g *Guard) RequireHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, g.addr) {
			writeJSON(w, http.StatusForbidden, ErrorResponse{Error: ErrorBody{Code: CodeForbiddenHost,
				Message: fmt.Sprintf("this server only answers requests for %s", g.addr), Details: map[string]any{}}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireToken lets a request through to the API only with the session
// token, either as "Authorization: Bearer <token>" or as the session cookie.
// GET /api/v1/version is the one route that needs neither. A request that
// changes state and is authenticated by the cookie must also carry the
// listener's own Origin, because a browser sends the cookie with requests
// from other pages on the same site, such as another port on 127.0.0.1.
//
// No Access-Control-Allow-Origin header is ever sent: the client UI is
// served from the same origin, and other origins must not read responses.
func (g *Guard) RequireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isOpenRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			scheme, token, ok := strings.Cut(auth, " ")
			if ok && strings.EqualFold(scheme, "Bearer") && g.session.Valid(strings.TrimSpace(token)) {
				next.ServeHTTP(w, r)
				return
			}
			g.unauthorized(w)
			return
		}
		c, err := r.Cookie(g.cookie)
		if err != nil || !g.session.Valid(c.Value) {
			g.unauthorized(w)
			return
		}
		if changesState(r.Method) && r.Header.Get("Origin") != g.origin {
			writeJSON(w, http.StatusForbidden, ErrorResponse{Error: ErrorBody{Code: CodeForbiddenOrigin,
				Message: "a request that changes state and is authenticated by the session cookie must come from " + g.origin,
				Details: map[string]any{}}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SignIn handles the sign-in URL on the client UI: a GET or HEAD whose
// query holds the token. With the right token it sets the session cookie,
// HttpOnly and SameSite=Strict, and redirects to the same path without the
// token, so the token leaves the address bar and the history. With a wrong
// one it answers 401. Other requests go to next.
func (g *Guard) SignIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !q.Has("token") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !g.session.Valid(q.Get("token")) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprintln(w, "This sign-in link is not valid. nebi prints a new link each time it starts; open the latest one.")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     g.cookie,
			Value:    g.session.token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		q.Del("token")
		target := signInRedirect(r.URL)
		if enc := q.Encode(); enc != "" {
			target += "?" + enc
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
	})
}

// signInRedirect is the path the sign-in URL redirects to: the request's own
// path, as it was escaped, so that /a%3Fb stays one path segment. A path
// that a browser could read as another host, such as //evil.example or
// /\evil.example, is replaced by /.
func signInRedirect(u *url.URL) string {
	target := u.EscapedPath()
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") || strings.HasPrefix(u.Path, "//") ||
		strings.Contains(target, `\`) || strings.Contains(u.Path, `\`) {
		return "/"
	}
	return target
}

func (g *Guard) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="nebi"`)
	writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: ErrorBody{Code: CodeUnauthorized,
		Message: "a valid session token is required", Details: map[string]any{}}})
}

// isOpenRoute reports whether r is GET /api/v1/version, the one route that
// needs no token.
func isOpenRoute(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == Prefix+"/version"
}

func changesState(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}
