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
