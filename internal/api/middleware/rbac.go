package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
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

// RequireWorkspaceAccess checks if user can access a workspace.
// In local mode, all access is granted without RBAC checks.
func RequireWorkspaceAccess(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Local mode: skip RBAC checks entirely
		if isLocal, _ := c.Get("is_local_mode"); isLocal == true {
			c.Next()
			return
		}

		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		wsIDStr := c.Param("id")
		wsID, err := uuid.Parse(wsIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
			c.Abort()
			return
		}

		userID := user.(*models.User).ID

		var hasAccess bool
		if action == "read" {
			hasAccess, err = rbac.CanReadWorkspace(userID, wsID)
		} else if action == "write" {
			hasAccess, err = rbac.CanWriteWorkspace(userID, wsID)
		}

		if err != nil || !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			c.Abort()
			return
		}

		c.Next()
	}
}
