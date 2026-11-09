// Package shared holds the types every client SDK package and its callers
// (internal/client/api, internal/client/jobs, cmd/nebi-cli) agree on:
// errors, dependencies, the job-submission interface, cross-unit hooks and
// the REST/CLI response types (issue #649). It imports no other
// internal/client package, so every SDK sub-package can import it.
//
// This package is a frozen contract: feature units do not edit it. See
// CONTRACTS.md for the process to change it.
package shared

import (
	"errors"
	"fmt"
)

// ErrNotImplemented is returned by stubs that a feature unit has not filled
// in yet. The client API maps it to HTTP 501.
var ErrNotImplemented = errors.New("not implemented")

// NotImplemented wraps ErrNotImplemented with the function name.
func NotImplemented(fn string) error {
	return fmt.Errorf("%s: %w", fn, ErrNotImplemented)
}

// ErrNotFound maps to HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrNotConnected is returned by remote operations when no server is
// connected. Maps to HTTP 409 with code "not_connected".
var ErrNotConnected = errors.New("no server connected")

// ErrUnavailable is returned when a capability is missing on this host or
// registry (e.g. no native folder picker, registry cannot delete tags).
// Maps to HTTP 501 with code "unavailable".
var ErrUnavailable = errors.New("unavailable")

// ValidationError is a user-correctable input problem. Maps to HTTP 422.
// Field names the offending request field ("" for the whole request).
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ConflictError is a state conflict. Maps to HTTP 409. Code is a stable
// machine-readable identifier the UI can branch on, e.g. "name_taken",
// "server_ahead", "job_active".
type ConflictError struct {
	Code    string
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

// RemoteError wraps an error returned by the connected team server. Status
// is the server's HTTP status (0 when unreachable). Maps to 502 when the
// server is unreachable and to Status otherwise.
type RemoteError struct {
	Status  int
	Message string
}

func (e *RemoteError) Error() string { return e.Message }
