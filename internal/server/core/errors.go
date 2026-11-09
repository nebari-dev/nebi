package core

import (
	"errors"
	"fmt"
)

// ErrNotImplemented marks stubs not filled in yet (HTTP 501).
var ErrNotImplemented = errors.New("not implemented")

func notImplemented(fn string) error { return fmt.Errorf("%s: %w", fn, ErrNotImplemented) }

// ErrNotFound maps to HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrForbidden maps to HTTP 403.
var ErrForbidden = errors.New("forbidden")

// ValidationError maps to HTTP 422.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ConflictError maps to HTTP 409 with a stable Code, e.g.
// "version_conflict" (a pushed version number exists with other content),
// "server_ahead" (the push skips versions the client does not have),
// "last_owner" (cannot remove the only owner).
type ConflictError struct {
	Code    string
	Message string
}

func (e *ConflictError) Error() string { return e.Message }
