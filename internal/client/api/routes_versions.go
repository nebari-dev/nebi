package api

// Owner: U2 (detail). Version rail, version detail, files, resolved
// packages, archive download.

import (
	"bytes"
	"errors"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
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
	ctx := c.Request.Context()
	v, err := a.SDK.Local.GetVersion(ctx, id, n)
	if errors.Is(err, shared.ErrNotFound) && a.projectExists(c, id) {
		// A server-only version (listed by the remote decorator).
		if sv, serr := a.SDK.Remote.ServerVersion(ctx, id, n); serr == nil {
			v, err = sv, nil
		} else if remoteMatters(serr) {
			err = serr
		}
	}
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
	ctx := c.Request.Context()
	name := c.Param("name")
	f, err := a.SDK.Local.GetVersionFile(ctx, id, n, name)
	if errors.Is(err, shared.ErrNotFound) && a.projectExists(c, id) {
		if toml, lock, serr := a.SDK.Remote.ServerVersionFiles(ctx, id, n); serr == nil {
			content := toml
			if name == artifact.LockFile {
				content = lock
			}
			f, err = &shared.FileContent{Name: name, Content: content}, nil
		} else if remoteMatters(serr) {
			err = serr
		}
	}
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
	ctx := c.Request.Context()
	p, err := a.SDK.Local.ResolvedPackages(ctx, id, n)
	if errors.Is(err, shared.ErrNotFound) && a.projectExists(c, id) {
		if toml, lock, serr := a.SDK.Remote.ServerVersionFiles(ctx, id, n); serr == nil {
			p, err = local.ResolvedPackagesFromFiles([]byte(toml), []byte(lock))
		} else if remoteMatters(serr) {
			err = serr
		}
	}
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
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Data(http.StatusOK, artifact.ArchiveContentType, buf.Bytes())
}

// projectExists reports whether the local project exists, so a missing
// version can be looked up on the linked server (a missing project stays
// a plain 404).
func (a *API) projectExists(c *gin.Context, id uuid.UUID) bool {
	_, err := a.SDK.Local.ResolveProject(c.Request.Context(), id.String())
	return err == nil
}

// remoteMatters reports whether a failed server lookup should replace the
// local "not found": the server answered with an error or did not answer.
// Not connected, not implemented and "not on the server" keep the 404.
func remoteMatters(err error) bool {
	if errors.Is(err, shared.ErrNotConnected) || errors.Is(err, shared.ErrNotImplemented) || errors.Is(err, shared.ErrNotFound) {
		return false
	}
	var rerr *shared.RemoteError
	if errors.As(err, &rerr) {
		return rerr.Status != http.StatusNotFound
	}
	return false
}
