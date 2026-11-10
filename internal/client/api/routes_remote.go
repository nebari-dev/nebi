package api

// Owner: U5 (remote). Server connection, server projects, pull, push,
// pull a server-only version.

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerRemoteRoutes(g *gin.RouterGroup) {
	g.GET("/remote/server", a.getServerConnection)
	g.PUT("/remote/server", a.connectServer)
	g.DELETE("/remote/server", a.disconnectServer)
	g.GET("/remote/projects", a.listServerProjects)
	g.POST("/remote/projects/refresh", a.refreshServerProjects)
	g.GET("/remote/projects/:serverProjectId", a.getRemoteDetails)
	g.POST("/remote/projects/:serverProjectId/pull", a.pullServerProject)
	g.POST("/projects/:id/push", a.pushProject)
	g.POST("/projects/:id/versions/:version/pull", a.pullServerVersion)
}

// getServerConnection: GET /client/remote/server -> shared.ServerConnection
// (connected=false when none)
func (a *API) getServerConnection(c *gin.Context) {
	conn, err := a.SDK.Remote.GetConnection(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, conn)
}

// connectServer: PUT /client/remote/server shared.ConnectRequest -> shared.ServerConnection
func (a *API) connectServer(c *gin.Context) {
	var req shared.ConnectRequest
	if !bindJSON(c, &req) {
		return
	}
	conn, err := a.SDK.Remote.Connect(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, conn)
}

// disconnectServer: DELETE /client/remote/server -> 204
func (a *API) disconnectServer(c *gin.Context) {
	if err := a.SDK.Remote.Disconnect(c.Request.Context()); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// listServerProjects: GET /client/remote/projects?search= -> shared.ServerProjectList
func (a *API) listServerProjects(c *gin.Context) {
	res, err := a.SDK.Remote.ListServerProjects(c.Request.Context(), c.Query("search"), false)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// refreshServerProjects: POST /client/remote/projects/refresh -> shared.ServerProjectList
func (a *API) refreshServerProjects(c *gin.Context) {
	res, err := a.SDK.Remote.ListServerProjects(c.Request.Context(), c.Query("search"), true)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getRemoteDetails: GET /client/remote/projects/:serverProjectId -> shared.RemoteDetails
func (a *API) getRemoteDetails(c *gin.Context) {
	res, err := a.SDK.Remote.GetRemoteDetails(c.Request.Context(), c.Param("serverProjectId"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// pullServerProject: POST /client/remote/projects/:serverProjectId/pull
// shared.PullRequest -> 202 shared.PullResult; 409 code "name_taken"
func (a *API) pullServerProject(c *gin.Context) {
	var req shared.PullRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Remote.Pull(c.Request.Context(), c.Param("serverProjectId"), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, res)
}

// pushRequest is the optional body of POST /client/projects/:id/push:
// up_to limits the push to versions numbered up to N ("Push to server" on
// version N). An empty body pushes every version not yet on the server.
type pushRequest struct {
	UpTo int `json:"up_to"`
}

// pushProject: POST /client/projects/:id/push [pushRequest] -> shared.PushResult
func (a *API) pushProject(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req pushRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: "bad_request"})
			return
		}
	}
	if req.UpTo < 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid up_to", Code: "bad_request", Field: "up_to"})
		return
	}
	res, err := a.SDK.Remote.PushUpTo(c.Request.Context(), id, req.UpTo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// pullServerVersion: POST /client/projects/:id/versions/:version/pull -> shared.VersionItem
func (a *API) pullServerVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	v, err := a.SDK.Remote.PullVersion(c.Request.Context(), id, n)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}
