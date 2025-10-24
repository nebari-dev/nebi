package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NotImplemented is a placeholder handler for unimplemented endpoints
func NotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not implemented",
		"message": "This endpoint will be implemented in future phases",
	})
}
