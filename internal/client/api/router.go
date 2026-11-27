package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

// Prefix is the path every client API route is under.
const Prefix = "/api/v1"

// handlerFunc is a route handler. It writes the response itself on
// success, and returns an error, without writing anything, on failure; the
// router turns the error into an ErrorResponse with the router's ErrorMap.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

// router dispatches /api/v1 requests to the handlers route groups register,
// and writes every error in the API's error format, including a route or
// method it does not have.
type router struct {
	mux    *http.ServeMux
	errors ErrorMap
	logger *slog.Logger
	// routes maps a mux pattern to its handlers by method.
	routes map[string]map[string]handlerFunc
}

func newRouter(logger *slog.Logger) *router {
	r := &router{mux: http.NewServeMux(), logger: logger, routes: map[string]map[string]handlerFunc{}}
	r.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		r.writeError(w, req, &Error{Status: http.StatusNotFound, Code: CodeNotFound,
			Message: fmt.Sprintf("no route %s", req.URL.Path)})
	})
	return r
}

// handle registers h for method and path, which is relative to Prefix and
// may hold net/http wildcards such as {id}. It panics when the route is
// registered twice.
func (r *router) handle(method, path string, h handlerFunc) {
	pattern := Prefix + path
	methods, ok := r.routes[pattern]
	if !ok {
		methods = map[string]handlerFunc{}
		r.routes[pattern] = methods
		r.mux.Handle(pattern, r.dispatch(methods))
	}
	if _, dup := methods[method]; dup {
		panic(fmt.Sprintf("api: route %s %s is registered twice", method, pattern))
	}
	methods[method] = h
}

// dispatch picks the handler for the request's method, so that a known path
// with an unknown method gets a 405 in the API's error format rather than
// net/http's plain text.
func (r *router) dispatch(methods map[string]handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		method := req.Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		h, ok := methods[method]
		if !ok {
			allowed := make([]string, 0, len(methods))
			for m := range methods {
				allowed = append(allowed, m)
			}
			slices.Sort(allowed)
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			r.writeError(w, req, &Error{Status: http.StatusMethodNotAllowed, Code: CodeMethodNotAllowed,
				Message: fmt.Sprintf("%s is not allowed on %s", req.Method, req.URL.Path)})
			return
		}
		if err := h(w, req); err != nil {
			r.writeError(w, req, err)
		}
	})
}

// ServeHTTP serves an API request. Responses are never cached: the UI polls
// and must see fresh data, and the desktop webview would otherwise cache
// them.
func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	defer func() {
		if v := recover(); v != nil {
			if v == http.ErrAbortHandler {
				panic(v) // net/http's way to abort a response; let it through
			}
			r.writeError(w, req, fmt.Errorf("panic: %v", v))
		}
	}()
	r.mux.ServeHTTP(w, req)
}

// writeError writes err as an ErrorResponse. Server errors are logged with
// the method and path, never the query, which can hold a token.
func (r *router) writeError(w http.ResponseWriter, req *http.Request, err error) {
	status, body := r.errors.Resolve(err)
	if status >= http.StatusInternalServerError {
		r.logger.Error("client API request failed", "method", req.Method, "path", req.URL.Path, "status", status, "error", err)
	}
	writeJSON(w, status, ErrorResponse{Error: body})
}

// writeJSON writes v as the JSON response body with status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		data, _ = json.Marshal(ErrorResponse{Error: ErrorBody{Code: CodeInternal, Message: "encoding the response failed", Details: map[string]any{}}})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}
