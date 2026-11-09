package api

// Owner: U2 (detail). Version rail, version detail, files, resolved
// packages, archive download.

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerVersionsRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/versions", a.listVersions)
	g.GET("/projects/:id/versions/:version", a.getVersion)
	g.GET("/projects/:id/versions/:version/files/:name", a.getVersionFile)
	g.GET("/projects/:id/versions/:version/packages", a.getResolvedPackages)
	g.GET("/projects/:id/versions/:version/archive", a.downloadArchive)
}

// listVersions: GET /client/projects/:id/versions -> []shared.VersionItem
// (newest first, server-only versions included)
func (a *API) listVersions(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	items, err := a.SDK.Local.ListVersions(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	if items == nil {
		items = []shared.VersionItem{}
	}
	c.JSON(http.StatusOK, items)
}

// getVersion: GET /client/projects/:id/versions/:version -> shared.VersionDetail
func (a *API) getVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	v, err := a.SDK.Local.GetVersion(c.Request.Context(), id, n)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

// getVersionFile: GET /client/projects/:id/versions/:version/files/:name
// (name: pixi.toml | pixi.lock) -> shared.FileContent
func (a *API) getVersionFile(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	f, err := a.SDK.Local.GetVersionFile(c.Request.Context(), id, n, c.Param("name"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, f)
}

// getResolvedPackages: GET /client/projects/:id/versions/:version/packages -> shared.ResolvedPackages
func (a *API) getResolvedPackages(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	p, err := a.SDK.Local.ResolvedPackages(c.Request.Context(), id, n)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// downloadArchive: GET /client/projects/:id/versions/:version/archive ->
// application/gzip attachment
func (a *API) downloadArchive(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n, ok := versionParam(c)
	if !ok {
		return
	}
	var buf bytes.Buffer
	name, err := a.SDK.Local.WriteVersionArchive(c.Request.Context(), id, n, &buf)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(http.StatusOK, "application/gzip", buf.Bytes())
}
