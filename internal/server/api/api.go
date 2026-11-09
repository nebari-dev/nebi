// Package api mounts the team-server endpoints added for issue #649 on the
// authenticated /api/v1 group of the shared router (team mode only). It
// must never import internal/client/....
//
// Each unit owns one routes_<unit>.go file with a register function already
// called from Register. Path parameter names match the legacy router so
// gin's tree never conflicts: :id (project UUID), :version, :kind,
// :principalId, :publicationId.
//
// File ownership: api.go is frozen.
package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/api/middleware"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/server/core"
)

// Deps are what the routes need.
type Deps struct {
	Core *core.Service
	RBAC rbac.Provider
	// LocalMode is false whenever Register is called; kept so the shared
	// RBAC middleware gets the same flag as the legacy routes.
	LocalMode bool
}

type routes struct {
	deps Deps
}

// Register mounts the routes on the authenticated /api/v1 group.
func Register(protected *gin.RouterGroup, deps Deps) {
	r := &routes{deps: deps}
	r.registerSyncRoutes(protected)
	r.registerSharingRoutes(protected)
	r.registerPublicationRoutes(protected)
}

// read and write are the per-project RBAC guards.
func (r *routes) read() gin.HandlerFunc {
	return middleware.RequireProjectAccess("read", r.deps.LocalMode, r.deps.RBAC)
}

func (r *routes) write() gin.HandlerFunc {
	return middleware.RequireProjectAccess("write", r.deps.LocalMode, r.deps.RBAC)
}

// ErrorResponse matches the legacy {"error": ...} body plus a stable code.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
	Field string `json:"field,omitempty"`
}

func writeError(c *gin.Context, err error) {
	var verr *core.ValidationError
	var cerr *core.ConflictError
	switch {
	case errors.As(err, &verr):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: verr.Message, Code: "invalid", Field: verr.Field})
	case errors.As(err, &cerr):
		c.JSON(http.StatusConflict, ErrorResponse{Error: cerr.Message, Code: cerr.Code})
	case errors.Is(err, core.ErrNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error(), Code: "not_found"})
	case errors.Is(err, core.ErrForbidden):
		c.JSON(http.StatusForbidden, ErrorResponse{Error: err.Error(), Code: "forbidden"})
	case errors.Is(err, core.ErrNotImplemented):
		c.JSON(http.StatusNotImplemented, ErrorResponse{Error: err.Error(), Code: "not_implemented"})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: "internal"})
	}
}

func bindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: "bad_request"})
		return false
	}
	return true
}

// userID is the authenticated caller.
func userID(c *gin.Context) uuid.UUID {
	if v, ok := c.Get(auth.UserContextKey); ok {
		if u, ok := v.(*models.User); ok {
			return u.ID
		}
	}
	return uuid.Nil
}

func uuidParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid " + name, Code: "bad_request", Field: name})
		return uuid.Nil, false
	}
	return id, true
}

func versionParam(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("version"))
	if err != nil || n < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid version", Code: "bad_request", Field: "version"})
		return 0, false
	}
	return n, true
}
