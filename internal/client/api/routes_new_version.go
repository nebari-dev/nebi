package api

// Owner: U3 (newversion).

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func (a *API) registerNewVersionRoutes(g *gin.RouterGroup) {
	g.POST("/projects/:id/versions", a.createVersion)
	g.POST("/projects/:id/versions/preview", a.previewVersion)
	g.GET("/projects/:id/versions/:version/draft", a.versionDraft)
}

// createVersion: POST /client/projects/:id/versions shared.CreateVersionRequest
// -> 202 shared.JobSummary (job.version_number is set when it completes).
// 422 {field} for invalid input, 409 code "no_changes" when the draft is
// identical to the base version.
func (a *API) createVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req shared.CreateVersionRequest
	if !bindJSON(c, &req) {
		return
	}
	job, err := a.SDK.Local.CreateVersion(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// previewRequest is the body of POST /versions/preview. Exactly one of
// PixiToml (pixi.toml editor) or Form (GUI form, applied onto the base
// version's pixi.toml) is set.
type previewRequest struct {
	BaseVersion int                  `json:"base_version"`
	PixiToml    string               `json:"pixi_toml"`
	Form        *shared.ManifestForm `json:"form,omitempty"`
}

// previewVersion: POST /client/projects/:id/versions/preview
// {"base_version", "pixi_toml"} or {"base_version", "form"} ->
// shared.VersionDiff (the "Summary of changes").
func (a *API) previewVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req previewRequest
	if !bindJSON(c, &req) {
		return
	}
	ctx := c.Request.Context()
	toml, err := a.SDK.Local.DraftManifest(ctx, id, req.BaseVersion, req.PixiToml, req.Form)
	if err != nil {
		writeError(c, err)
		return
	}
	res, err := a.SDK.Local.PreviewChanges(ctx, id, req.BaseVersion, toml)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// versionDraft: GET /client/projects/:id/versions/:version/draft ->
// local.VersionDraft, the starting point of a new version from :version
// ("latest" = the newest local version): pixi.toml, the form, latest
// version, newer versions and the project's tags.
func (a *API) versionDraft(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	n := 0 // "latest"
	if c.Param("version") != "latest" {
		var ok bool
		if n, ok = versionParam(c); !ok {
			return
		}
	}
	res, err := a.SDK.Local.GetVersionDraft(c.Request.Context(), id, n)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
