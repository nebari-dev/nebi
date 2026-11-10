package api

// Owner: U4 (compare).

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (a *API) registerCompareRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/compare", a.compareVersions)
}

// compareVersions: GET /client/projects/:id/compare?base=N&target=M ->
// shared.CompareResult (either side may be a server-only version)
func (a *API) compareVersions(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	base, err1 := strconv.Atoi(c.Query("base"))
	target, err2 := strconv.Atoi(c.Query("target"))
	if err1 != nil || err2 != nil || base < 1 || target < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "base and target must be version numbers", Code: "bad_request"})
		return
	}
	res, err := a.SDK.Compare(c.Request.Context(), id, base, target)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
