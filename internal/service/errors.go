package service

import "errors"

// ErrNotFound indicates the requested resource was not found.
var ErrNotFound = errors.New("not found")

// ValidationError represents a bad-request condition (HTTP 400).
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ConflictError represents a conflict condition (HTTP 409).
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

// ForbiddenError represents a forbidden condition (HTTP 403).
type ForbiddenError struct {
	Message string
}

func (e *ForbiddenError) Error() string { return e.Message }

// NotFoundError represents a not-found condition (HTTP 404) that carries a
// caller-facing message. It matches ErrNotFound under errors.Is, so code
// that only checks the sentinel keeps working.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string { return e.Message }

// Is reports whether target is ErrNotFound.
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// UnprocessableError represents a well-formed request that names something
// the server cannot process, such as an OCI artifact that is not a valid
// Nebi bundle (HTTP 422).
type UnprocessableError struct {
	Message string
}

func (e *UnprocessableError) Error() string { return e.Message }

// UpstreamError represents a refusal by an upstream service the server
// called on the caller's behalf, such as an OCI registry denying access
// (HTTP 502).
//
// Every field is safe to return to the caller and to log. The upstream's
// own error is deliberately not carried: it can hold request URLs with
// signed query strings and response bodies that echo credentials.
type UpstreamError struct {
	Message string
	// UpstreamStatus is the HTTP status the upstream answered with.
	UpstreamStatus int
	// Op is the server-side operation that was refused, for logs.
	Op string
	// Target names what was being accessed (for a registry,
	// "host/repository"), for logs.
	Target string
}

func (e *UpstreamError) Error() string { return e.Message }
