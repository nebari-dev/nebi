package oci

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"oras.land/oras-go/v2/registry/remote/auth"
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
// A refusal by a host the request was merely redirected to (for example
// a blob storage backend) is not reported as a RegistryAccessError.
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

// redirectedResponseError stands in for a 401, 403 or 404 that was
// answered by a host the request had been redirected to.
type redirectedResponseError struct{ statusCode int }

func (e *redirectedResponseError) Error() string {
	return fmt.Sprintf("redirected to another host, which answered %d %s", e.statusCode, http.StatusText(e.statusCode))
}

// originTransport makes sure the statuses a bundle pull is classified by
// (401, 403, 404) were answered by the host the request was addressed
// to. When a request has been redirected to another host (a blob storage
// backend, say) and that host answers with one of them, the status says
// nothing about whether the registry has, or grants access to, the
// reference. RoundTrip then returns an error instead of the response, so
// the registry client reports a plain transport failure.
//
// The decision is made per request, from the redirect chain net/http
// records on the request itself. Nothing is remembered between requests.
type originTransport struct {
	base http.RoundTripper // nil means http.DefaultTransport
}

func (t *originTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		if redirectedToAnotherHost(req) {
			resp.Body.Close()
			return nil, &redirectedResponseError{statusCode: resp.StatusCode}
		}
	}
	return resp, nil
}

// redirectedToAnotherHost reports whether req is a redirect follow-up
// whose host differs from the host of the request that started the
// chain. net/http sets Request.Response on every request it creates to
// follow a redirect.
func redirectedToAnotherHost(req *http.Request) bool {
	first := req
	for first.Response != nil && first.Response.Request != nil {
		first = first.Response.Request
	}
	return !strings.EqualFold(first.URL.Host, req.URL.Host)
}

// classifyAccess wraps err in a RegistryAccessError when the registry or
// its token service refused the pull: a 401 or 403 error response, or a
// Basic challenge (a 401) with no credentials configured to answer it.
// Both are read from the registry client's typed errors, not from error
// text. The message of err is unchanged and err stays in the chain.
func classifyAccess(err error) error {
	if errors.Is(err, auth.ErrBasicCredentialNotFound) {
		return &RegistryAccessError{StatusCode: http.StatusUnauthorized, err: err}
	}
	var resp *errcode.ErrorResponse
	if errors.As(err, &resp) &&
		(resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		return &RegistryAccessError{StatusCode: resp.StatusCode, err: err}
	}
	return err
}
