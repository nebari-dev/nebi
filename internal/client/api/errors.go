package api

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
)

// ErrorResponse is the body of every error response of the client API:
//
//	{"error": {"code": "name_taken", "message": "project name is already taken", "details": {}}}
//
// Clients branch on Code, which is stable and part of the API contract,
// never on Message, which is for people and may change.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is the "error" object of an ErrorResponse.
type ErrorBody struct {
	// Code names the error, such as "name_taken". Stable.
	Code string `json:"code" example:"name_taken"`
	// Message describes the error for people.
	Message string `json:"message" example:"project name is already taken"`
	// Details carries extra data for the error, such as the IDs of the jobs
	// that keep a project busy. Always present; empty when there is none.
	Details map[string]any `json:"details"`
}

// Codes of the errors the API itself returns. Resource route groups add
// their own when they register their SDK errors.
const (
	CodeNotFound                = "not_found"
	CodeMethodNotAllowed        = "method_not_allowed"
	CodeUnauthorized            = "unauthorized"
	CodeForbiddenHost           = "forbidden_host"
	CodeForbiddenOrigin         = "forbidden_origin"
	CodeInternal                = "internal_error"
	CodeFolderPickerUnavailable = "folder_picker_unavailable"
	CodeWindowUnavailable       = "window_unavailable"
	// CodeWindowNotReady: the desktop app has a window, but it is not open,
	// because it is still starting or has closed.
	CodeWindowNotReady = "window_not_ready"
)

// Error is an error a handler returns with its status and code set, for
// failures that are not an SDK error, such as a route the process does not
// have. The router writes it as it is.
type Error struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string { return e.Message }

// ErrorMapping maps one typed error, matched with errors.Is, to an HTTP
// status and a code.
type ErrorMapping struct {
	// Err is the error to match, usually an SDK sentinel such as
	// local.ErrNameTaken. Required.
	Err error
	// Status is the HTTP status, 400 to 599. Required.
	Status int
	// Code is the stable code, in snake_case. Required.
	Code string
	// Details returns the details of a matched error, such as the job IDs
	// of a *local.BusyError, found with errors.As. Optional.
	Details func(err error) map[string]any
}

// ErrorMap maps the errors handlers return to statuses and codes. Route
// groups register their errors while New builds the handler; after that
// the map is only read, so it needs no lock.
type ErrorMap struct {
	mappings []ErrorMapping
}

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// Register adds mappings. The first registered mapping whose Err matches
// wins, so register a more specific error before one it wraps. Register
// panics on an invalid mapping or on an error registered twice: both are
// programming errors that every test of the handler would hit.
func (m *ErrorMap) Register(mappings ...ErrorMapping) {
	for _, mp := range mappings {
		switch {
		case mp.Err == nil:
			panic("api: ErrorMapping.Err is nil")
		case mp.Status < 400 || mp.Status > 599:
			panic(fmt.Sprintf("api: error %q maps to status %d, want 400 to 599", mp.Err, mp.Status))
		case !codePattern.MatchString(mp.Code):
			panic(fmt.Sprintf("api: error %q maps to code %q, want snake_case", mp.Err, mp.Code))
		}
		for _, have := range m.mappings {
			if have.Err == mp.Err {
				panic(fmt.Sprintf("api: error %q is registered twice", mp.Err))
			}
		}
		m.mappings = append(m.mappings, mp)
	}
}

// Resolve returns the status and body for err. An *Error is used as it is;
// otherwise the first registered mapping that matches err with errors.Is
// gives the status and code, and the message is err's text. Any other
// error is a 500 with a generic message, so internal detail does not leak;
// the caller logs it.
func (m *ErrorMap) Resolve(err error) (int, ErrorBody) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Status, ErrorBody{Code: apiErr.Code, Message: apiErr.Message, Details: nonNil(apiErr.Details)}
	}
	for _, mp := range m.mappings {
		if !errors.Is(err, mp.Err) {
			continue
		}
		var details map[string]any
		if mp.Details != nil {
			details = mp.Details(err)
		}
		return mp.Status, ErrorBody{Code: mp.Code, Message: err.Error(), Details: nonNil(details)}
	}
	return http.StatusInternalServerError, ErrorBody{Code: CodeInternal, Message: "internal error", Details: map[string]any{}}
}

func nonNil(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	return details
}
