package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse is a standard error response
type ErrorResponse struct {
	Error string `json:"error"`
	// UpstreamStatus is the HTTP status an upstream service (such as an
	// OCI registry) answered with. Set only on 502 responses.
	UpstreamStatus int `json:"upstream_status,omitempty"`
}

// NotImplemented is a placeholder handler for unimplemented endpoints
func NotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not implemented",
		"message": "This endpoint will be implemented in future phases",
	})
}
