package api

// Owner: U7 (publish). Publication metadata pushed by clients.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/server/core"
)

func (r *routes) registerPublicationRoutes(g *gin.RouterGroup) {
	g.POST("/projects/:id/sync/versions/:version/publications", r.write(), r.recordPublication)
	g.DELETE("/projects/:id/sync/versions/:version/publications/:publicationId", r.write(), r.removePublication)
}

// recordPublication: POST /api/v1/projects/:id/sync/versions/:version/publications
// core.PublicationRecord -> 201 core.PublicationRecord
func (r *routes) recordPublication(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	var req core.PublicationRecord
	if !bindJSON(c, &req) {
		return
	}
	res, err := r.deps.Core.RecordPublication(c.Request.Context(), userID(c), id, n, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// removePublication: DELETE /api/v1/projects/:id/sync/versions/:version/publications/:publicationId -> 204
func (r *routes) removePublication(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	pubID, ok := uuidParam(c, "publicationId")
	if !ok {
		return
	}
	if err := r.deps.Core.RemovePublication(c.Request.Context(), userID(c), id, n, pubID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
