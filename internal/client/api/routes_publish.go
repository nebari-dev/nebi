package api

// Owner: U7 (publish). Registries, publish, unpublish, pull from registry.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/registry"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerPublishRoutes(g *gin.RouterGroup) {
	g.GET("/registries", a.listRegistries)
	g.POST("/registries", a.addRegistry)
	g.PUT("/registries/:registryId", a.updateRegistry)
	g.DELETE("/registries/:registryId", a.removeRegistry)
	g.GET("/registries/:registryId/repositories", a.listRegistryRepositories)
	g.GET("/registries/:registryId/tags", a.listRegistryTags)
	g.POST("/registries/:registryId/pull", a.pullFromRegistry)
	g.GET("/projects/:id/versions/:version/publish-defaults", a.publishDefaults)
	g.POST("/projects/:id/versions/:version/publish", a.publishVersion)
	g.DELETE("/projects/:id/publications/:publicationId", a.unpublish)
}

// listRegistries: GET /client/registries -> []shared.RegistryItem
func (a *API) listRegistries(c *gin.Context) {
	res, err := a.SDK.Registry.ListRegistries(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []shared.RegistryItem{}
	}
	c.JSON(http.StatusOK, res)
}

// addRegistry: POST /client/registries shared.AddRegistryRequest -> 201 shared.RegistryItem
func (a *API) addRegistry(c *gin.Context) {
	var req shared.AddRegistryRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Registry.AddRegistry(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// updateRegistry: PUT /client/registries/:registryId
// registry.UpdateRegistryRequest -> shared.RegistryItem
func (a *API) updateRegistry(c *gin.Context) {
	id, ok := uuidParam(c, "registryId")
	if !ok {
		return
	}
	var req registry.UpdateRegistryRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Registry.UpdateRegistry(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// removeRegistry: DELETE /client/registries/:registryId -> 204
func (a *API) removeRegistry(c *gin.Context) {
	id, ok := uuidParam(c, "registryId")
	if !ok {
		return
	}
	if err := a.SDK.Registry.RemoveRegistry(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// listRegistryRepositories: GET /client/registries/:registryId/repositories?search= -> []string
func (a *API) listRegistryRepositories(c *gin.Context) {
	id, ok := uuidParam(c, "registryId")
	if !ok {
		return
	}
	res, err := a.SDK.Registry.ListRepositories(c.Request.Context(), id, c.Query("search"))
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []string{}
	}
	c.JSON(http.StatusOK, res)
}

// listRegistryTags: GET /client/registries/:registryId/tags?repository= -> []string
func (a *API) listRegistryTags(c *gin.Context) {
	id, ok := uuidParam(c, "registryId")
	if !ok {
		return
	}
	res, err := a.SDK.Registry.ListTags(c.Request.Context(), id, c.Query("repository"))
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []string{}
	}
	c.JSON(http.StatusOK, res)
}

// pullFromRegistry: POST /client/registries/:registryId/pull
// shared.RegistryPullRequest -> 202 shared.PullResult
func (a *API) pullFromRegistry(c *gin.Context) {
	id, ok := uuidParam(c, "registryId")
	if !ok {
		return
	}
	var req shared.RegistryPullRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Registry.Pull(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, res)
}

// publishDefaults: GET /client/projects/:id/versions/:version/publish-defaults -> []shared.PublishDefault
func (a *API) publishDefaults(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	res, err := a.SDK.Registry.PublishDefaults(c.Request.Context(), id, n)
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []shared.PublishDefault{}
	}
	c.JSON(http.StatusOK, res)
}

// publishVersion: POST /client/projects/:id/versions/:version/publish
// shared.PublishRequest -> shared.PublishResult (200 even when some
// targets failed; inspect results[].status)
func (a *API) publishVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	var req shared.PublishRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Registry.Publish(c.Request.Context(), id, n, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// unpublish: DELETE /client/projects/:id/publications/:publicationId -> 204;
// 501 code "unavailable" when the registry cannot delete tags
func (a *API) unpublish(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	pubID, ok := uuidParam(c, "publicationId")
	if !ok {
		return
	}
	if err := a.SDK.Registry.Unpublish(c.Request.Context(), id, pubID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
