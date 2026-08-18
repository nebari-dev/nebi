package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxRequestBodyBytes caps request bodies before any JSON binding happens.
func MaxRequestBodyBytes(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
