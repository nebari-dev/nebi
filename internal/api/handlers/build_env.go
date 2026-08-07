package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/service"
)

// BuildEnvHandler handles current-user build environment variables.
type BuildEnvHandler struct {
	svc *service.WorkspaceService
}

// NewBuildEnvHandler creates a new build environment handler.
func NewBuildEnvHandler(svc *service.WorkspaceService) *BuildEnvHandler {
	return &BuildEnvHandler{svc: svc}
}

// ListBuildEnvVars godoc
// @Summary List build environment variables
// @Description Returns the current user's configured build environment variable metadata. Values are never returned.
// @Tags build-env-vars
// @Produce json
// @Security BearerAuth
// @Success 200 {array} service.BuildEnvVarResult
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /build-env-vars [get]
func (h *BuildEnvHandler) ListBuildEnvVars(c *gin.Context) {
	vars, err := h.svc.ListBuildEnvVars(getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, vars)
}

// UpsertBuildEnvVar godoc
// @Summary Create or update a build environment variable
// @Description Stores a current-user build environment variable. Values are encrypted at rest and are never returned.
// @Tags build-env-vars
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body BuildEnvVarRequest true "Build environment variable"
// @Success 200 {object} service.BuildEnvVarResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /build-env-vars [put]
func (h *BuildEnvHandler) UpsertBuildEnvVar(c *gin.Context) {
	var req BuildEnvVarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.svc.UpsertBuildEnvVar(getUserID(c), service.BuildEnvVarReq{
		Key:   req.Key,
		Value: req.Value,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteBuildEnvVar godoc
// @Summary Delete a build environment variable
// @Description Removes a current-user build environment variable by key.
// @Tags build-env-vars
// @Security BearerAuth
// @Param key path string true "Environment variable key"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /build-env-vars/{key} [delete]
func (h *BuildEnvHandler) DeleteBuildEnvVar(c *gin.Context) {
	if err := h.svc.DeleteBuildEnvVar(getUserID(c), c.Param("key")); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// BuildEnvVarRequest is used to create or update a build variable.
type BuildEnvVarRequest struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value" binding:"required"`
}
