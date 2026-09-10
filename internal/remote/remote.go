// Package remote is a SPIKE (see issue: make nebi servers and OCI
// registries peers). It defines a single Remote interface that a client
// (CLI, or the web UI in NEBI_MODE=local) can program against without
// knowing whether the other end is a nebi server or an OCI registry,
// plus one implementation per backend:
//
//   - OCIRemote    (oci.go)    — wraps internal/oci / oras
//   - ServerRemote (server.go) — wraps internal/cliclient
//
// The point of the exercise is to find out where the interface proposed
// in the issue is sufficient and where it leaks. Deviations from the
// issue's sketch and everything learned along the way are written up in
// README.md next to this file.
package remote

import (
	"context"
	"errors"
	"strings"
	"time"
)

// AuthenticationCredentialType names an authentication scheme a remote
// understands.
type AuthenticationCredentialType string

const (
	BasicAuthenticationCredentialType AuthenticationCredentialType = "basic"
	TokenAuthenticationCredentialType AuthenticationCredentialType = "token"
	// DeviceAuthenticationCredentialType marks a *dynamic* scheme: an
	// interactive flow (RFC 8628 device authorization grant) that
	// cannot be expressed as a static AuthenticationCredential. A
	// remote advertising it must also implement DeviceAuthenticator;
	// see the static/dynamic split documented on that interface.
	DeviceAuthenticationCredentialType AuthenticationCredentialType = "device"
)

// AuthenticationCredential carries the user-supplied secret for one of
// the schemes above. Exactly the shape from the issue.
type AuthenticationCredential struct {
	Type     AuthenticationCredentialType
	Username string
	Password string
	Token    string
}

func (ac *AuthenticationCredential) Validate() error {
	switch ac.Type {
	case BasicAuthenticationCredentialType:
		if ac.Username == "" || ac.Password == "" {
			return errors.New("basic auth requires username and password")
		}
	case TokenAuthenticationCredentialType:
		if ac.Token == "" {
			return errors.New("token auth requires token")
		}
	default:
		return errors.New("unknown auth type")
	}
	return nil
}

// Sentinel errors both implementations map their backend-native
// failures into, so a client can react uniformly with errors.Is.
var (
	// ErrNotAuthenticated: a method that needs credentials was called
	// before a successful Authenticate.
	ErrNotAuthenticated = errors.New("remote: not authenticated")
	// ErrUnauthorized: the remote rejected the credentials.
	ErrUnauthorized = errors.New("remote: unauthorized")
	// ErrForbidden: authenticated, but no permission for the operation.
	ErrForbidden = errors.New("remote: forbidden")
	// ErrNotFound: the workspace (or the requested ref) does not exist.
	ErrNotFound = errors.New("remote: workspace not found")
	// ErrConflict: the push would clobber an existing version ref.
	ErrConflict = errors.New("remote: version already exists")
)

// WorkspacePreview is the listing entry for one workspace on a remote.
//
// Deviation from the issue's sketch: `Version uint` is replaced by
// string refs. An OCI registry has no monotonic version numbers — a
// version is identified by a tag (nebi's default tag is the content
// hash) — so a numeric version cannot be satisfied by every remote.
// The server maps its tags into refs; its numeric version numbers stay
// a backend detail.
type WorkspacePreview struct {
	// ID is the remote-native identifier: a workspace UUID on a nebi
	// server, the namespace-relative repository name on an OCI
	// registry. Opaque to the client; feed it back to Pull.
	ID   string
	Name string
	// Refs are the version refs (tags) known for this workspace,
	// newest first where the backend can tell.
	Refs []string
	// LatestRef is the ref Pull(id) without an explicit ref resolves
	// to. Empty when the remote has no versions yet.
	LatestRef string
}

// Workspace is one concrete version of a workspace: the preview plus
// the content that defines the environment. Assets beyond the two core
// files exist on the OCI side but are deliberately out of scope for the
// spike (they are listed, never fetched).
type Workspace struct {
	WorkspacePreview
	// Ref is the version ref this content belongs to (on Pull) or the
	// ref to create (on Push).
	Ref      string
	PixiToml string
	PixiLock string
	// AssetPaths lists bundle files beyond pixi.toml/pixi.lock, when
	// the backend has them (OCI bundles). Listing only; content is not
	// carried.
	AssetPaths []string
}

// Remote is the issue's proposed interface, with two adjustments:
// every method takes a context (both backends are network clients),
// and Pull's id may carry an optional "@<ref>" suffix to address a
// specific version (default: latest).
type Remote interface {
	// SupportedAuthentication lists the schemes Authenticate accepts.
	// Empty means the remote needs no authentication (e.g. a public
	// read-only OCI registry) and Authenticate may be skipped.
	SupportedAuthentication() []AuthenticationCredentialType
	// Authenticate verifies the credential against the remote. On
	// error the client should refuse to add the remote.
	Authenticate(ctx context.Context, ac AuthenticationCredential) error
	// List returns every workspace the authenticated user can read.
	List(ctx context.Context) ([]WorkspacePreview, error)
	// Pull fetches one workspace version. id is a WorkspacePreview.ID,
	// optionally suffixed with "@<ref>"; without a ref the latest
	// version is returned.
	Pull(ctx context.Context, id string) (Workspace, error)
	// Push uploads a new workspace or a new version of an existing
	// one. ws.Ref names the version being pushed; pushing a ref that
	// already exists fails with ErrConflict.
	Push(ctx context.Context, ws Workspace) error
}

// Authentication is split between static and dynamic schemes:
//
//   - Static schemes (basic, token) are a credential the user can type
//     up front; they go through Remote.Authenticate.
//   - Dynamic schemes require an interaction in the middle of the
//     flow — the client must show the user a code and a URL, then wait
//     for the user to approve in a browser. That can never fit in an
//     AuthenticationCredential struct, so it is expressed as the
//     optional DeviceAuthenticator extension below.
//
// A client authenticates a remote like this:
//
//	switch scheme chosen from r.SupportedAuthentication() {
//	case basic, token:
//	    r.Authenticate(ctx, cred)
//	case device:
//	    da := r.(remote.DeviceAuthenticator)   // advertised ⇒ implemented
//	    auth, _ := da.BeginDeviceAuthentication(ctx)
//	    // show auth.UserCode and auth.VerificationURI to the user
//	    da.CompleteDeviceAuthentication(ctx, auth) // blocks until approved
//	}
//
// DeviceAuthenticator is implemented by remotes that support the RFC
// 8628 device authorization grant. A remote only advertises
// DeviceAuthenticationCredentialType in SupportedAuthentication when it
// implements this interface.
type DeviceAuthenticator interface {
	// BeginDeviceAuthentication starts the flow and returns the prompt
	// the client must show the user.
	BeginDeviceAuthentication(ctx context.Context) (DeviceAuthorization, error)
	// CompleteDeviceAuthentication blocks, polling the identity
	// provider, until the user approves the prompt (then the remote is
	// authenticated, as after a successful Authenticate), the
	// authorization expires, or ctx is cancelled.
	CompleteDeviceAuthentication(ctx context.Context, da DeviceAuthorization) error
}

// DeviceAuthorization is the user-facing half of a pending device
// flow: everything the client needs to render the "go here, enter this
// code" prompt. The unexported fields are opaque continuation state a
// client just hands back to CompleteDeviceAuthentication.
type DeviceAuthorization struct {
	// UserCode is the short code the user enters at VerificationURI.
	UserCode string
	// VerificationURI is the page where the user approves the login.
	VerificationURI string
	// VerificationURIComplete, when non-empty, embeds the code in the
	// URL so the user only has to confirm (optional per RFC 8628).
	VerificationURIComplete string
	// ExpiresAt is when the pending authorization stops being valid.
	ExpiresAt time.Time

	deviceCode string        // RFC 8628 device_code being polled
	tokenURL   string        // issuer token endpoint to poll
	clientID   string        // OAuth client id the flow was started for
	interval   time.Duration // issuer-requested polling interval
}

// splitIDRef splits "id@ref" into its parts; ref is empty when absent.
func splitIDRef(id string) (string, string) {
	if i := strings.LastIndex(id, "@"); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}
