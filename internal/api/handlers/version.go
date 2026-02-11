package handlers

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
)

// Version is set via ldflags at build time
var Version = "dev"

// Mode is set by the router based on config (e.g. "local" or "team")
var Mode = "team"

// GetVersion godoc
// @Summary Get version information
// @Description Returns version information about the Nebi server
// @Tags system
// @Produce json
// @Success 200 {object} map[string]string
// @Router /version [get]
func GetVersion(c *gin.Context) {
	features := map[string]bool{
		"auth":          Mode != "local",
		"rbac":          Mode != "local",
		"remote_proxy":  Mode == "local",
		"local_storage": Mode == "local",
	}

	c.JSON(http.StatusOK, gin.H{
		"version":    Version,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"mode":       Mode,
		"features":   features,
	})
}
