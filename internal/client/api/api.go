// Package api is the client REST API (issue #680): the client SDK
// projected over HTTP under /api/v1. The process that owns the job runner
// for a client database serves it on loopback, behind a session token, for
// the client UI and for CLI commands that find another process owning the
// database.
//
// New builds the handler from its dependencies. It opens no database,
// starts no job runner and listens on nothing; the composition roots
// (internal/client/web and internal/client/desktop, through
// internal/client/boot) own those. Guard protects the handler on a loopback
// listener: it checks the Host header, the session token and, for requests
// authenticated by cookie, the Origin.
//
// Each resource registers its routes and its errors from its own file; see
// routeGroups. Every error response has the body ErrorResponse describes.
//
// The OpenAPI spec of this API is generated from the annotations in this
// package into internal/client/api/openapi by `make swagger`, separately
// from the team server's spec in internal/swagger.
//
//	@title						Nebi client API
//	@version					1.0
//	@description				The API of the nebi client process (nebi-web or the desktop app) that owns the client database. It listens on 127.0.0.1 only, and answers 403 forbidden_host to a request whose Host header is not its own address. Every route except GET /version needs the session token, as a bearer token or as the session cookie the sign-in URL sets.
//	@BasePath					/api/v1
//	@securityDefinitions.apikey	SessionToken
//	@in							header
//	@name						Authorization
//	@description				"Bearer " followed by the session token from owner.json. A browser sends the token instead as the session cookie nebi_session_<port>, which the sign-in URL sets (OpenAPI 2.0 cannot describe a cookie scheme). A request authenticated by that cookie that changes state must carry the server's own Origin, or it gets 403 forbidden_origin.
package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
)

// ClientAPIVersion is the version of the client API this build serves, as
// "major.minor". It is written to owner.json and returned by GET /version.
//
// Adding an endpoint or a field raises the minor version; a breaking change
// raises the major version and resets the minor. A client works with an
// owner that has the same major version and an equal or higher minor
// version; the CLI checks this before it sends a command to another process
// (#664).
const ClientAPIVersion = "1.0"

// Mode is the runtime mode GET /version reports. The client API is only
// served in local mode.
const Mode = "local"

// VersionInfo is the build information GET /version reports.
type VersionInfo struct {
	// NebiVersion is the nebi release, such as "0.15.0" or "dev". Required.
	NebiVersion string
	// Commit is the git commit the binary was built from. Optional.
	Commit string
}

// Options are the dependencies of the client API handler.
type Options struct {
	// Service is the local project SDK. Required.
	Service *local.Service
	// Jobs is the job runner the Service submits to. Required.
	Jobs *jobs.Memory
	// Logger receives server errors. Nil means slog.Default().
	Logger *slog.Logger
	// Version is reported by GET /version. NebiVersion is required.
	Version VersionInfo
	// FolderPicker shows a native folder dialog. Only the desktop app has
	// one; nil means POST /desktop/pick-folder answers 404 and
	// GET /capabilities reports folder_picker false.
	FolderPicker FolderPicker
	// Window is the desktop app's window. Only the desktop app has one; nil
	// means POST /desktop/show-window answers 404.
	Window Window
}

// New returns the client API handler. It serves /api/v1 and answers every
// other path with a 404 in the API's error format. It does not check the
// session token: the desktop app's own window reaches it in-process, and a
// loopback listener wraps it with Guard.RequireToken.
func New(opts Options) (http.Handler, error) {
	switch {
	case opts.Service == nil:
		return nil, errors.New("api: Options.Service is required")
	case opts.Jobs == nil:
		return nil, errors.New("api: Options.Jobs is required")
	case opts.Version.NebiVersion == "":
		return nil, errors.New("api: Options.Version.NebiVersion is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	r := newRouter(opts.Logger)
	for _, register := range routeGroups {
		register(r, &opts)
	}
	return r, nil
}
