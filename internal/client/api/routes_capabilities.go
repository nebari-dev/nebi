package api

// Owner: U1 (projects). Host capabilities and the native folder picker.

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerCapabilitiesRoutes(g *gin.RouterGroup) {
	g.GET("/capabilities", a.getCapabilities)
	g.POST("/desktop/pick-folder", a.pickFolder)
}

// getCapabilities: GET /client/capabilities -> shared.Capabilities
func (a *API) getCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, shared.Capabilities{
		NativeFolderPicker: a.Desktop.FolderPicker != nil,
		Platform:           runtime.GOOS + "/" + runtime.GOARCH,
		DefaultProjectsDir: a.SDK.Deps().DefaultProjectsDir(),
	})
}

// pickFolder: POST /client/desktop/pick-folder shared.PickFolderRequest ->
// shared.PickFolderResult; 501 code "unavailable" without a native picker.
func (a *API) pickFolder(c *gin.Context) {
	if a.Desktop.FolderPicker == nil {
		writeError(c, shared.ErrUnavailable)
		return
	}
	var req shared.PickFolderRequest
	if c.Request.ContentLength != 0 && !bindJSON(c, &req) {
		return
	}
	path, err := a.Desktop.FolderPicker.PickFolder(c.Request.Context(), req.Title, req.DefaultPath)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.PickFolderResult{Path: path, Cancelled: path == ""})
}
