package api

// Owner: U5 (remote). Catalog and version sync.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/server/core"
)

func (r *routes) registerSyncRoutes(g *gin.RouterGroup) {
	g.GET("/catalog/projects", r.listCatalogProjects)
	g.POST("/catalog/projects", r.createCatalogProject)
	g.GET("/projects/:id/sync/versions", r.read(), r.listSyncVersions)
	g.GET("/projects/:id/sync/versions/:version", r.read(), r.getSyncVersion)
	g.POST("/projects/:id/sync/versions", r.write(), r.pushVersions)
}

// listCatalogProjects: GET /api/v1/catalog/projects?search= -> []core.CatalogProject
func (r *routes) listCatalogProjects(c *gin.Context) {
	res, err := r.deps.Core.ListCatalogProjects(c.Request.Context(), userID(c), c.Query("search"))
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []core.CatalogProject{}
	}
	c.JSON(http.StatusOK, res)
}

// createCatalogProject: POST /api/v1/catalog/projects {"name"} -> 201 core.CatalogProject
func (r *routes) createCatalogProject(c *gin.Context) {
	var req core.CreateCatalogProjectRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := r.deps.Core.CreateCatalogProject(c.Request.Context(), userID(c), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// listSyncVersions: GET /api/v1/projects/:id/sync/versions -> []core.SyncVersion
func (r *routes) listSyncVersions(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	res, err := r.deps.Core.ListSyncVersions(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []core.SyncVersion{}
	}
	c.JSON(http.StatusOK, res)
}

// getSyncVersion: GET /api/v1/projects/:id/sync/versions/:version -> core.SyncVersionFiles
func (r *routes) getSyncVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	res, err := r.deps.Core.GetSyncVersion(c.Request.Context(), id, n)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// pushVersions: POST /api/v1/projects/:id/sync/versions core.PushVersionsRequest
// -> core.PushVersionsResult; 409 code "version_conflict" | "server_ahead"
func (r *routes) pushVersions(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req core.PushVersionsRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := r.deps.Core.PushVersions(c.Request.Context(), userID(c), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
