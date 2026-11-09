// Package api is the client REST API (issue #649): the local-mode HTTP
// surface of the client SDK for frontend/apps/client. Every route lives
// under /api/v1/client and is mounted only in local mode (nebi-web, the
// desktop app), behind the local authenticator.
//
// Each unit owns one routes_<unit>.go file with a register<Unit>Routes
// method already called from Register; units add routes and handlers only
// in their own files. Path parameter names are fixed so gin's routing tree
// never conflicts across files: :id (local project UUID), :version
// (version number), :name (file name), :jobId, :registryId,
// :publicationId, :serverProjectId, :kind, :principalId.
//
// File ownership: api.go is frozen.
package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// API serves the client REST API.
type API struct {
	SDK     *sdk.Client
	Jobs    *jobs.Service
	Desktop desktop.Capabilities
}

// New builds the client API.
func New(c *sdk.Client, j *jobs.Service, d desktop.Capabilities) *API {
	return &API{SDK: c, Jobs: j, Desktop: d}
}

// Register mounts every client route on the authenticated /api/v1 group.
func (a *API) Register(protected *gin.RouterGroup) {
	g := protected.Group("/client")
	a.registerCapabilitiesRoutes(g)
	a.registerProjectsRoutes(g)
	a.registerManifestRoutes(g)
	a.registerVersionsRoutes(g)
	a.registerExecRoutes(g)
	a.registerNewVersionRoutes(g)
	a.registerCompareRoutes(g)
	a.registerRemoteRoutes(g)
	a.registerSharingRoutes(g)
	a.registerPublishRoutes(g)
	a.registerJobsRoutes(g)
}

// ErrorResponse is the body of every client API error. Code is a stable
// identifier ("not_implemented", "not_connected", "name_taken",
// "server_unreachable", ...); Field names the invalid request field.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
	Field string `json:"field,omitempty"`
}

// writeError maps SDK errors to HTTP responses.
func writeError(c *gin.Context, err error) {
	var verr *shared.ValidationError
	var cerr *shared.ConflictError
	var rerr *shared.RemoteError
	switch {
	case errors.As(err, &verr):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: verr.Message, Code: "invalid", Field: verr.Field})
	case errors.As(err, &cerr):
		c.JSON(http.StatusConflict, ErrorResponse{Error: cerr.Message, Code: cerr.Code})
	case errors.As(err, &rerr):
		status, code := remoteStatus(rerr.Status)
		c.JSON(status, ErrorResponse{Error: rerr.Message, Code: code})
	case errors.Is(err, shared.ErrNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error(), Code: "not_found"})
	case errors.Is(err, shared.ErrNotConnected):
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error(), Code: "not_connected"})
	case errors.Is(err, shared.ErrUnavailable):
		c.JSON(http.StatusNotImplemented, ErrorResponse{Error: err.Error(), Code: "unavailable"})
	case errors.Is(err, shared.ErrNotImplemented):
		c.JSON(http.StatusNotImplemented, ErrorResponse{Error: err.Error(), Code: "not_implemented"})
	default:
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: err.Error(), Code: "too_large"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: "internal"})
	}
}

// remoteStatus maps a team-server status to the client API status. A 401
// from the server must not look like a local 401.
func remoteStatus(status int) (int, string) {
	switch status {
	case 0:
		return http.StatusBadGateway, "server_unreachable"
	case http.StatusUnauthorized:
		return http.StatusBadGateway, "server_auth"
	case http.StatusForbidden:
		return http.StatusForbidden, "server_forbidden"
	case http.StatusNotFound:
		return http.StatusNotFound, "server_not_found"
	case http.StatusConflict:
		return http.StatusConflict, "server_conflict"
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return http.StatusUnprocessableEntity, "server_invalid"
	default:
		return http.StatusBadGateway, "server_error"
	}
}

// bindJSON decodes the body into v, writing a 400/413 and returning false
// on failure.
func bindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: err.Error(), Code: "too_large"})
			return false
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: "bad_request"})
		return false
	}
	return true
}

// uuidParam parses a UUID path parameter, writing a 400 on failure.
func uuidParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid " + name, Code: "bad_request", Field: name})
		return uuid.Nil, false
	}
	return id, true
}

// versionParam parses the :version path parameter.
func versionParam(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("version"))
	if err != nil || n < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid version", Code: "bad_request", Field: "version"})
		return 0, false
	}
	return n, true
}
