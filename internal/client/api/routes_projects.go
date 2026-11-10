package api

// Owner: U1 (projects).

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerProjectsRoutes(g *gin.RouterGroup) {
	g.GET("/projects", a.listProjects)
	g.POST("/projects", a.createProject)
	g.GET("/projects/name-check", a.checkProjectName)
	g.GET("/projects/:id", a.getProject)
	g.DELETE("/projects/:id", a.deleteProject)
}

// listProjects: GET /client/projects -> []shared.ProjectListItem
func (a *API) listProjects(c *gin.Context) {
	items, err := a.SDK.Local.ListProjects(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	if items == nil {
		items = []shared.ProjectListItem{}
	}
	c.JSON(http.StatusOK, items)
}

// createProject: POST /client/projects shared.CreateProjectRequest ->
// 202 shared.CreateProjectResult
func (a *API) createProject(c *gin.Context) {
	var req shared.CreateProjectRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Local.CreateProject(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, res)
}

// checkProjectName: GET /client/projects/name-check?name= -> shared.NameCheckResult
func (a *API) checkProjectName(c *gin.Context) {
	res, err := a.SDK.Local.CheckName(c.Request.Context(), c.Query("name"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getProject: GET /client/projects/:id -> shared.ProjectListItem
func (a *API) getProject(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	item, err := a.SDK.Local.GetProject(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// deleteProject: DELETE /client/projects/:id -> 202 shared.JobSummary
func (a *API) deleteProject(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	job, err := a.SDK.Local.DeleteProject(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}
