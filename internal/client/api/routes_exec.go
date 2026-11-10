package api

// Owner: U2 (detail). Install / uninstall (also used by the projects list).

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerExecRoutes(g *gin.RouterGroup) {
	g.POST("/projects/:id/install", a.installVersion)
	g.POST("/projects/:id/uninstall", a.uninstall)
}

// installVersion: POST /client/projects/:id/install shared.InstallRequest
// (empty body = last installed, else latest) -> 202 shared.JobSummary
func (a *API) installVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req shared.InstallRequest
	if c.Request.ContentLength != 0 && !bindJSON(c, &req) {
		return
	}
	job, err := a.SDK.Exec.InstallVersion(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// uninstall: POST /client/projects/:id/uninstall -> 202 shared.JobSummary
func (a *API) uninstall(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	job, err := a.SDK.Exec.Uninstall(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}
