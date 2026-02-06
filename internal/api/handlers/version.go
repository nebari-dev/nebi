package handlers

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
)

// Version is set via ldflags at build time
var Version = "dev"

// Mode is set by the router based on config
var Mode = "team"

// GetVersion godoc
// @Summary Get version information
// @Description Returns version information about the Nebi server
// @Tags system
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /version [get]
func GetVersion(c *gin.Context) {
	isLocal := Mode == "local"
	c.JSON(http.StatusOK, gin.H{
		"version":    Version,
		"mode":       Mode,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"features": gin.H{
			"authentication": !isLocal,
			"userManagement": !isLocal,
			"auditLogs":      !isLocal,
		},
	})
}
