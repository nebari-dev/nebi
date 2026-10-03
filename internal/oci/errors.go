package oci

import (
	"errors"
	"net/http"

	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// Sentinel errors for bundle pulls. PullBundle and ExtractBundle wrap
// them, so callers classify a failure with errors.Is instead of matching
// message text.
var (
	// ErrNotNebiArtifact reports that the reference resolved to an OCI
	// artifact that is not a Nebi bundle (for example an ordinary
	// container image).
	ErrNotNebiArtifact = errors.New("not a Nebi artifact")

	// ErrInvalidBundle reports that the artifact claims to be a Nebi
	// bundle but its manifest is malformed: a missing or duplicate core
	// layer, a core layer with the wrong title, an unknown layer media
	// type, or an unsafe asset path.
	ErrInvalidBundle = errors.New("invalid bundle")

	// ErrReferenceNotFound reports that the registry does not have the
	// requested repository or tag.
	ErrReferenceNotFound = errors.New("repository or tag not found")
)

// RegistryStatus returns the HTTP status code a registry (or its token
// service) answered with, when err carries one. It reads the status from
// the registry client's typed error response rather than from the error
// text. A "not found" the client reports without a response body is
// returned as 404.
func RegistryStatus(err error) (int, bool) {
	var resp *errcode.ErrorResponse
	if errors.As(err, &resp) {
		return resp.StatusCode, true
	}
	if errors.Is(err, errdef.ErrNotFound) {
		return http.StatusNotFound, true
	}
	return 0, false
}
