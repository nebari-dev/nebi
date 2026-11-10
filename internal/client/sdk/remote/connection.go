package remote

// Owner: U5 (remote). One server at a time; auth none or token.

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// AnonymousUser is shown as "Signed in as" for connections without a token
// that the server does not identify.
const AnonymousUser = "anonymous"

// hostPattern is the prototype's URL check, applied to the URL without
// its scheme ("nebi.example.com", "localhost:8460/nebi").
var hostPattern = regexp.MustCompile(`(?i)^[a-z0-9.-]+(:\d+)?(/.*)?$`)

// GetConnection returns the current connection ("Signed in as").
func (s *Service) GetConnection(ctx context.Context) (*shared.ServerConnection, error) {
	c, err := s.local.LoadServerCredentials(ctx)
	if err != nil {
		return nil, err
	}
	return connectionFrom(c), nil
}

func connectionFrom(c *local.ServerCredentials) *shared.ServerConnection {
	if c == nil || c.URL == "" {
		return &shared.ServerConnection{Auth: local.AuthModeNone}
	}
	return &shared.ServerConnection{
		Connected:    true,
		URL:          c.URL,
		Name:         c.Name,
		Username:     c.Username,
		Auth:         c.AuthMode,
		LastSyncedAt: c.LastSyncedAt,
	}
}

// Connect verifies the server (GET /api/v1/version and /auth/me with the
// token) and stores the connection, replacing any previous one. With auth
// "token" and no token, the stored token is kept when the URL is unchanged
// ("Manage connection" saves without retyping it).
func (s *Service) Connect(ctx context.Context, req shared.ConnectRequest) (*shared.ServerConnection, error) {
	base, err := NormalizeServerURL(req.URL)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(req.Token)
	mode := strings.TrimSpace(req.Auth)
	switch mode {
	case "":
		mode = local.AuthModeNone
		if token != "" {
			mode = local.AuthModeToken
		}
	case local.AuthModeNone:
		token = ""
	case local.AuthModeToken:
	default:
		return nil, &shared.ValidationError{Field: "auth", Message: "Choose None or Token."}
	}
	if mode == local.AuthModeToken && token == "" {
		prev, err := s.local.LoadServerCredentials(ctx)
		if err != nil {
			return nil, err
		}
		if prev.URL == base && prev.Token != "" {
			token = prev.Token
		} else {
			return nil, &shared.ValidationError{Field: "token", Message: "Paste a token, or choose None."}
		}
	}

	var info serverVersionInfo
	if err := s.doJSON(ctx, base, "", http.MethodGet, "/version", nil, &info); err != nil {
		var rerr *shared.RemoteError
		if errors.As(err, &rerr) {
			return nil, &shared.ValidationError{Field: "url", Message: "Couldn’t reach a Nebi server at " + displayURL(base) + "."}
		}
		return nil, err
	}
	if info.Mode == "local" {
		return nil, &shared.ValidationError{Field: "url", Message: displayURL(base) + " is a single-user Nebi, not a team server."}
	}

	username := ""
	var me serverUser
	if err := s.doJSON(ctx, base, token, http.MethodGet, "/auth/me", nil, &me); err != nil {
		var rerr *shared.RemoteError
		switch {
		case errors.As(err, &rerr) && rerr.Status == http.StatusUnauthorized && mode == local.AuthModeToken:
			return nil, &shared.ValidationError{Field: "token", Message: "The server didn’t accept this token."}
		case errors.As(err, &rerr) && (rerr.Status == http.StatusUnauthorized || rerr.Status == http.StatusForbidden):
			username = AnonymousUser
		case errors.As(err, &rerr):
			return nil, &shared.ValidationError{Field: "url", Message: "Couldn’t reach a Nebi server at " + displayURL(base) + "."}
		default:
			return nil, err
		}
	} else {
		username = me.Username
		if username == "" {
			username = me.Email
		}
	}
	if username == "" {
		username = AnonymousUser
	}

	if err := s.local.SaveServerCredentials(ctx, local.ServerCredentials{
		URL:      base,
		Name:     ServerName(base),
		Token:    token,
		Username: username,
		AuthMode: mode,
	}); err != nil {
		return nil, err
	}
	resetServerCache()
	return s.GetConnection(ctx)
}

// Disconnect forgets the server. Pulled projects stay and keep working:
// their project_remotes rows are kept so they re-link on reconnect.
func (s *Service) Disconnect(ctx context.Context) error {
	if err := s.local.ClearServerCredentials(ctx); err != nil {
		return err
	}
	resetServerCache()
	return nil
}

// NormalizeServerURL turns user input ("nebi.example.com",
// "https://nebi.example.com/") into the stored base URL. A missing scheme
// means https. Errors are field "url" validation errors with prototype copy.
func NormalizeServerURL(raw string) (string, error) {
	in := strings.TrimSpace(raw)
	if in == "" {
		return "", &shared.ValidationError{Field: "url", Message: "Enter the server’s URL."}
	}
	invalid := &shared.ValidationError{Field: "url", Message: "That doesn’t look like a URL. Try nebi.example.com."}
	lower := strings.ToLower(in)
	scheme := "https"
	switch {
	case strings.HasPrefix(lower, "https://"):
		in = in[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		scheme, in = "http", in[len("http://"):]
	case strings.Contains(in, "://"):
		return "", invalid
	}
	in = strings.TrimRight(in, "/")
	if !hostPattern.MatchString(in) {
		return "", invalid
	}
	u, err := url.Parse(scheme + "://" + in)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", invalid
	}
	return scheme + "://" + u.Host + strings.TrimRight(u.EscapedPath(), "/"), nil
}

// ServerName is the display label of a server URL: its host (and port).
func ServerName(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return displayURL(base)
	}
	return u.Host
}

// displayURL drops the scheme ("nebi.example.com").
func displayURL(base string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	return strings.TrimRight(s, "/")
}
