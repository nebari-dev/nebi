package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// RequireAdmin ensures the user is an admin.
// When localMode is true the check is unconditionally skipped.
func RequireAdmin(localMode bool) gin.HandlerFunc {
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
// When localMode is true the check is unconditionally skipped.
// It first checks Casbin RBAC policies (individual access), then falls back
// to checking group-based permissions in the database.
func RequireWorkspaceAccess(action string, localMode bool, db *gorm.DB) gin.HandlerFunc {
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

		wsIDStr := c.Param("id")
		wsID, err := uuid.Parse(wsIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
			c.Abort()
			return
		}

		u := user.(*models.User)
		userID := u.ID

		// Check individual RBAC access first
		var hasAccess bool
		if action == "read" {
			hasAccess, err = rbac.CanReadWorkspace(userID, wsID)
		} else if action == "write" {
			hasAccess, err = rbac.CanWriteWorkspace(userID, wsID)
		}

		if err == nil && hasAccess {
			c.Next()
			return
		}

		// Fallback: check group-based permissions
		if len(u.Groups) > 0 && db != nil {
			if checkGroupAccess(db, u.Groups, wsID, action) {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		c.Abort()
	}
}

// checkGroupAccess checks whether any of the user's groups have the required
// access level to the workspace via GroupPermission records.
func checkGroupAccess(db *gorm.DB, groups []string, wsID uuid.UUID, action string) bool {
	var groupPerms []models.GroupPermission
	if err := db.Preload("Role").
		Where("group_name IN ? AND workspace_id = ?", groups, wsID).
		Find(&groupPerms).Error; err != nil {
		return false
	}

	for _, gp := range groupPerms {
		switch action {
		case "read":
			// Any role grants read access
			if gp.Role.Name == "viewer" || gp.Role.Name == "editor" || gp.Role.Name == "owner" {
				return true
			}
		case "write":
			// Only editor and owner grant write access
			if gp.Role.Name == "editor" || gp.Role.Name == "owner" {
				return true
			}
		}
	}

	return false
}
