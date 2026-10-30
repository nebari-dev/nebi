package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
)

// RequireAdmin ensures the user is an admin.
// When localMode is true the check is unconditionally skipped.
func RequireAdmin(localMode bool, provider rbac.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if localMode {
			c.Next()
			return
		}

		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		userID := user.(*models.User).ID
		isAdmin, err := provider.IsAdmin(userID)
		if err != nil || !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireProjectAccess checks if user can access a project.
// When localMode is true the check is unconditionally skipped.
func RequireProjectAccess(action string, localMode bool, provider rbac.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if localMode {
			c.Next()
			return
		}

		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		projectIDStr := c.Param("id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
			c.Abort()
			return
		}

		userID := user.(*models.User).ID

		var hasAccess bool
		if action == "read" {
			hasAccess, err = provider.CanReadProject(userID, projectID)
		} else if action == "write" {
			hasAccess, err = provider.CanWriteProject(userID, projectID)
		}

		if err != nil || !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			c.Abort()
			return
		}

		c.Next()
	}
}
