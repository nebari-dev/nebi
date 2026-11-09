package api

// Owner: U3 (newversion).

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerNewVersionRoutes(g *gin.RouterGroup) {
	g.POST("/projects/:id/versions", a.createVersion)
	g.POST("/projects/:id/versions/preview", a.previewVersion)
}

// createVersion: POST /client/projects/:id/versions shared.CreateVersionRequest
// -> 202 shared.JobSummary (job.version_number is set when it completes)
func (a *API) createVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req shared.CreateVersionRequest
	if !bindJSON(c, &req) {
		return
	}
	job, err := a.SDK.Local.CreateVersion(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// previewVersion: POST /client/projects/:id/versions/preview
// {"base_version", "pixi_toml"} -> shared.VersionDiff
func (a *API) previewVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		BaseVersion int    `json:"base_version"`
		PixiToml    string `json:"pixi_toml"`
	}
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Local.PreviewChanges(c.Request.Context(), id, req.BaseVersion, req.PixiToml)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
