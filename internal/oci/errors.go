package oci

import (
	"errors"
	"net/http"
	"net/url"
	"sync"

	"oras.land/oras-go/v2/registry/remote/errcode"
)

// Sentinel errors for bundle pulls. PullBundle and ExtractBundle return
// errors that match them under errors.Is, so callers classify a failure
// without matching message text.
var (
	// ErrNotNebiArtifact reports that the reference resolved to an OCI
	// artifact that is not a Nebi bundle (for example an ordinary
	// container image).
	ErrNotNebiArtifact = errors.New("not a Nebi artifact")

	// ErrInvalidBundle reports that the artifact claims to be a Nebi
	// bundle but is malformed. The concrete error is an
	// *InvalidBundleError.
	ErrInvalidBundle = errors.New("invalid bundle")

	// ErrReferenceNotFound reports that the registry does not have the
	// requested repository or tag.
	ErrReferenceNotFound = errors.New("repository or tag not found")
)

// InvalidBundleError describes why an artifact was rejected as a Nebi
// bundle. It matches ErrInvalidBundle under errors.Is.
//
// Error() is the full diagnostic and can quote values taken from the
// manifest (layer titles, media types, asset paths), which the registry
// controls. Reason is a fixed description that quotes nothing from the
// manifest; use it wherever the text is shown to someone other than the
// operator who chose the registry.
type InvalidBundleError struct {
	Reason string

	detail string
	cause  error
}

func (e *InvalidBundleError) Error() string { return e.detail }

func (e *InvalidBundleError) Unwrap() error { return e.cause }

// Is reports whether target is ErrInvalidBundle.
func (e *InvalidBundleError) Is(target error) bool { return target == ErrInvalidBundle }

// RegistryAccessError reports that the registry, or the token service it
// delegates authentication to, answered a bundle pull with 401 or 403.
// A refusal by a host the registry merely redirected to (for example a
// blob storage backend) is not reported as a RegistryAccessError.
//
// Error() is the underlying client error and can contain request URLs
// and the response body, so it must not be shown to API callers.
type RegistryAccessError struct {
	// StatusCode is the status the registry or token service answered
	// with: 401 or 403.
	StatusCode int

	err error
}

func (e *RegistryAccessError) Error() string { return e.err.Error() }

func (e *RegistryAccessError) Unwrap() error { return e.err }

// referenceNotFoundError marks a tag-resolution failure as
// ErrReferenceNotFound without changing its message.
type referenceNotFoundError struct{ err error }

func (e *referenceNotFoundError) Error() string { return e.err.Error() }

func (e *referenceNotFoundError) Unwrap() error { return e.err }

func (e *referenceNotFoundError) Is(target error) bool { return target == ErrReferenceNotFound }

// maxRedirects matches net/http's default redirect limit.
const maxRedirects = 10

// redirectLog records, for one bundle pull, every redirect that left the
// registry's own host. A response that came back from such a redirect was
// produced by some other server, so its status says nothing about whether
// the registry has, or grants access to, the requested reference.
type redirectLog struct {
	registryHost string

	mu      sync.Mutex
	foreign map[string]struct{}
}

func newRedirectLog(registryHost string) *redirectLog {
	return &redirectLog{registryHost: registryHost, foreign: make(map[string]struct{})}
}

// checkRedirect is an http.Client.CheckRedirect hook. It keeps the
// default redirect limit and notes redirects to another host.
func (l *redirectLog) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("stopped after 10 redirects")
	}
	if req.URL.Host != l.registryHost {
		l.mu.Lock()
		l.foreign[req.URL.String()] = struct{}{}
		l.mu.Unlock()
	}
	return nil
}

// leftRegistry reports whether any request so far was redirected to
// another host.
func (l *redirectLog) leftRegistry() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.foreign) > 0
}

// redirectedTo reports whether u was reached by a redirect to another
// host.
func (l *redirectLog) redirectedTo(u *url.URL) bool {
	if u == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.foreign[u.String()]
	return ok
}

// classifyAccess wraps err in a RegistryAccessError when it carries a 401
// or 403 answered by the registry or its token service. The status and
// the answering URL come from the registry client's typed error response,
// not from the error text. The message of err is unchanged.
func (l *redirectLog) classifyAccess(err error) error {
	var resp *errcode.ErrorResponse
	if !errors.As(err, &resp) {
		return err
	}
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return err
	}
	if l.redirectedTo(resp.URL) {
		return err
	}
	return &RegistryAccessError{StatusCode: resp.StatusCode, err: err}
}
