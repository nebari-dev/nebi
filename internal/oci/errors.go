package oci

import (
	"errors"
	"net/http"

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

// RegistryAccessError reports that a bundle pull was refused with 401 or
// 403. The refusal came from the registry or from something the registry
// delegates to: its token service, or a host it redirected the request
// to (a blob storage backend, say). The error does not say which.
//
// Error() is the underlying client error and can contain request URLs
// and the response body, so it must not be shown to API callers.
type RegistryAccessError struct {
	// StatusCode is the status the pull was refused with: 401 or 403.
	StatusCode int

	err error
}

func (e *RegistryAccessError) Error() string { return e.err.Error() }

func (e *RegistryAccessError) Unwrap() error { return e.err }

// referenceNotFoundError marks a failure to resolve the tag or fetch its
// manifest (including the fetch oras.Copy makes) as ErrReferenceNotFound
// without changing its message.
type referenceNotFoundError struct{ err error }

func (e *referenceNotFoundError) Error() string { return e.err.Error() }

func (e *referenceNotFoundError) Unwrap() error { return e.err }

func (e *referenceNotFoundError) Is(target error) bool { return target == ErrReferenceNotFound }

// classifyAccess wraps err in a RegistryAccessError when the pull was
// refused: the registry client's typed error response carries a 401 or
// 403, or the registry sent a Basic challenge (a 401) and no credentials
// are configured to answer it. Nothing is read from error text. The
// message of err is unchanged and err stays in the chain.
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
