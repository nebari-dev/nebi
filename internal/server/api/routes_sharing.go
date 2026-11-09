package api

// Owner: U6 (sharing). Owner checks happen in core (like the legacy
// /share routes); reading the list needs read access.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/server/core"
)

func (r *routes) registerSharingRoutes(g *gin.RouterGroup) {
	g.GET("/principals", r.searchPrincipals)
	g.GET("/projects/:id/access", r.read(), r.listAccess)
	g.POST("/projects/:id/access", r.grantAccess)
	g.PUT("/projects/:id/access/:kind/:principalId", r.updateAccess)
	g.DELETE("/projects/:id/access/:kind/:principalId", r.revokeAccess)
}

// searchPrincipals: GET /api/v1/principals?q= -> []core.Principal
func (r *routes) searchPrincipals(c *gin.Context) {
	res, err := r.deps.Core.SearchPrincipals(c.Request.Context(), c.Query("q"))
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []core.Principal{}
	}
	c.JSON(http.StatusOK, res)
}

// listAccess: GET /api/v1/projects/:id/access -> core.AccessList
func (r *routes) listAccess(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	res, err := r.deps.Core.ListAccess(c.Request.Context(), userID(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// grantAccess: POST /api/v1/projects/:id/access core.GrantAccessRequest -> core.AccessList
func (r *routes) grantAccess(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req core.GrantAccessRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := r.deps.Core.GrantAccess(c.Request.Context(), userID(c), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// updateAccess: PUT /api/v1/projects/:id/access/:kind/:principalId
// core.UpdateAccessRequest -> core.AccessList
func (r *routes) updateAccess(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req core.UpdateAccessRequest
	if !bindJSON(c, &req) {
		return
	}
	p := core.PrincipalRef{Kind: c.Param("kind"), ID: c.Param("principalId")}
	res, err := r.deps.Core.UpdateAccess(c.Request.Context(), userID(c), id, p, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// revokeAccess: DELETE /api/v1/projects/:id/access/:kind/:principalId -> core.AccessList
func (r *routes) revokeAccess(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	p := core.PrincipalRef{Kind: c.Param("kind"), ID: c.Param("principalId")}
	res, err := r.deps.Core.RevokeAccess(c.Request.Context(), userID(c), id, p)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
