package api

// Owner: U6 (sharing). Share dialog in the client, proxied to the server.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerSharingRoutes(g *gin.RouterGroup) {
	g.GET("/remote/principals", a.searchPrincipals)
	g.GET("/remote/projects/:serverProjectId/access", a.listAccess)
	g.POST("/remote/projects/:serverProjectId/access", a.grantAccess)
	g.PUT("/remote/projects/:serverProjectId/access/:kind/:principalId", a.updateAccess)
	g.DELETE("/remote/projects/:serverProjectId/access/:kind/:principalId", a.revokeAccess)
}

// searchPrincipals: GET /client/remote/principals?q= -> []shared.Principal
func (a *API) searchPrincipals(c *gin.Context) {
	res, err := a.SDK.Remote.SearchPrincipals(c.Request.Context(), c.Query("q"))
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []shared.Principal{}
	}
	c.JSON(http.StatusOK, res)
}

// listAccess: GET /client/remote/projects/:serverProjectId/access -> shared.AccessList
func (a *API) listAccess(c *gin.Context) {
	res, err := a.SDK.Remote.ListAccess(c.Request.Context(), c.Param("serverProjectId"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// grantAccess: POST /client/remote/projects/:serverProjectId/access
// shared.GrantAccessRequest -> shared.AccessList
func (a *API) grantAccess(c *gin.Context) {
	var req shared.GrantAccessRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Remote.GrantAccess(c.Request.Context(), c.Param("serverProjectId"), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// updateAccess: PUT /client/remote/projects/:serverProjectId/access/:kind/:principalId
// shared.UpdateAccessRequest -> shared.AccessList
func (a *API) updateAccess(c *gin.Context) {
	var req shared.UpdateAccessRequest
	if !bindJSON(c, &req) {
		return
	}
	p := shared.PrincipalRef{Kind: c.Param("kind"), ID: c.Param("principalId")}
	res, err := a.SDK.Remote.UpdateAccess(c.Request.Context(), c.Param("serverProjectId"), p, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// revokeAccess: DELETE /client/remote/projects/:serverProjectId/access/:kind/:principalId
// -> shared.AccessList
func (a *API) revokeAccess(c *gin.Context) {
	p := shared.PrincipalRef{Kind: c.Param("kind"), ID: c.Param("principalId")}
	res, err := a.SDK.Remote.RevokeAccess(c.Request.Context(), c.Param("serverProjectId"), p)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
