package handlers

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
)

// Version is set via ldflags at build time
var Version = "dev"

// GetVersion godoc
// @Summary Get version information
// @Description Returns version information about the Darb server
// @Tags system
// @x-cli true
// @Produce json
// @Success 200 {object} map[string]string
// @Router /version [get]
func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    Version,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	})
}
