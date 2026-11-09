package api

// Owner: U1 (projects). Form <-> pixi.toml round trip, used by Create
// project (U1) and New version (U3).

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerManifestRoutes(g *gin.RouterGroup) {
	g.POST("/manifest/parse", a.parseManifest)
	g.POST("/manifest/render", a.renderManifest)
}

// parseManifest: POST /client/manifest/parse {"pixi_toml"} -> shared.ManifestParseResult
func (a *API) parseManifest(c *gin.Context) {
	var req struct {
		PixiToml string `json:"pixi_toml"`
	}
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Local.ParseManifest(c.Request.Context(), req.PixiToml)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// renderManifest: POST /client/manifest/render shared.ManifestRenderRequest -> shared.ManifestRenderResult
func (a *API) renderManifest(c *gin.Context) {
	var req shared.ManifestRenderRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := a.SDK.Local.RenderManifest(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
