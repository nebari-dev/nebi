package middleware

import (
	"net/http"

	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/rbac"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireAdmin ensures the user is an admin
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		userID := user.(*models.User).ID
		isAdmin, err := rbac.IsAdmin(userID)
		if err != nil || !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireEnvironmentAccess checks if user can access an environment
func RequireEnvironmentAccess(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		envIDStr := c.Param("id")
		envID, err := uuid.Parse(envIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid environment ID"})
			c.Abort()
			return
		}

		userID := user.(*models.User).ID

		var hasAccess bool
		if action == "read" {
			hasAccess, err = rbac.CanReadEnvironment(userID, envID)
		} else if action == "write" {
			hasAccess, err = rbac.CanWriteEnvironment(userID, envID)
		}

		if err != nil || !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			c.Abort()
			return
		}

		c.Next()
	}
}
