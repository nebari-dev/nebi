package api

// Owner: U6 (sharing). Owner/admin checks happen in core (like the legacy
// /share routes). Reading the list needs read access, ownership or admin;
// core checks that too because admins have no per-project RBAC policy.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/server/core"
)

func (r *routes) registerSharingRoutes(g *gin.RouterGroup) {
	g.GET("/principals", r.searchPrincipals)
	g.GET("/projects/:id/access", r.listAccess)
	g.POST("/projects/:id/access", r.grantAccess)
	g.PUT("/projects/:id/access/:kind/:principalId", r.updateAccess)
	g.DELETE("/projects/:id/access/:kind/:principalId", r.revokeAccess)
	// Delete from the server UI (owner/admin only, unlike the legacy
	// DELETE /projects/:id which any editor may call).
	g.DELETE("/catalog/projects/:id", r.deleteServerProject)
}

// searchPrincipals: GET /api/v1/principals?q= -> []core.Principal
func (r *routes) searchPrincipals(c *gin.Context) {
	res, err := r.deps.Core.SearchPrincipals(c.Request.Context(), userID(c), c.Query("q"))
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

// deleteServerProject: DELETE /api/v1/catalog/projects/:id -> 202 (a server
// job removes the project); 403 unless owner or admin.
func (r *routes) deleteServerProject(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	if err := r.deps.Core.DeleteProject(c.Request.Context(), userID(c), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}
