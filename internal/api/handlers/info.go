package handlers

import (
	"net/http"
	"runtime"

	"github.com/aktech/darb/internal/db"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InfoHandler handles server info requests
type InfoHandler struct {
	db *gorm.DB
}

// NewInfoHandler creates a new InfoHandler
func NewInfoHandler(database *gorm.DB) *InfoHandler {
	return &InfoHandler{db: database}
}

// InfoResponse represents the server info response
type InfoResponse struct {
	ServerID  string `json:"server_id"`
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// GetInfo godoc
// @Summary Get server information
// @Description Returns server information including the unique server ID and version
// @Tags system
// @Produce json
// @Success 200 {object} InfoResponse
// @Failure 500 {object} ErrorResponse
// @Router /info [get]
func (h *InfoHandler) GetInfo(c *gin.Context) {
	serverID, err := db.GetServerID(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to retrieve server ID",
		})
		return
	}

	c.JSON(http.StatusOK, InfoResponse{
		ServerID:  serverID,
		Version:   Version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	})
}
