package handlers

import (
	"fmt"
	"net/http"

	"github.com/openteams-ai/darb/internal/audit"
	"github.com/openteams-ai/darb/internal/models"
	"github.com/openteams-ai/darb/internal/rbac"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AdminHandler struct {
	db *gorm.DB
}

func NewAdminHandler(db *gorm.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

// ListUsers godoc
// @Summary List all users (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} UserWithAdminStatus
// @Router /admin/users [get]
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var users []models.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch users"})
		return
	}

	// Get all admin user IDs in ONE Casbin call
	adminUserIDs, err := rbac.GetAllAdminUserIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check admin status"})
		return
	}

	// Build response with admin status using O(1) map lookup
	usersWithStatus := make([]UserWithAdminStatus, len(users))
	for i, user := range users {
		usersWithStatus[i] = UserWithAdminStatus{
			User:    user,
			IsAdmin: adminUserIDs[user.ID],
		}
	}

	c.JSON(http.StatusOK, usersWithStatus)
}

// CreateUser godoc
// @Summary Create a new user (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param user body CreateUserRequest true "User details"
// @Success 201 {object} models.User
// @Router /admin/users [post]
func (h *AdminHandler) CreateUser(c *gin.Context) {
	adminUser := c.MustGet("user").(*models.User)

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to hash password"})
		return
	}

	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create user"})
		return
	}

	// If admin flag is set, grant admin permissions
	if req.IsAdmin {
		if err := rbac.MakeAdmin(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to grant admin permissions"})
			return
		}
	}

	// Audit log
	audit.LogAction(h.db, adminUser.ID, audit.ActionCreateUser, "user:"+user.ID.String(), map[string]interface{}{
		"username": user.Username,
		"email":    user.Email,
		"is_admin": req.IsAdmin,
	})

	c.JSON(http.StatusCreated, user)
}

// GetUser godoc
// @Summary Get user by ID (admin only)
// @Tags admin
// @Security BearerAuth
// @Param id path string true "User UUID"
// @Success 200 {object} UserWithAdminStatus
// @Router /admin/users/{id} [get]
func (h *AdminHandler) GetUser(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	// Check if user is admin
	isAdmin, _ := rbac.IsAdmin(user.ID)

	c.JSON(http.StatusOK, UserWithAdminStatus{
		User:    user,
		IsAdmin: isAdmin,
	})
}

// ToggleAdmin godoc
// @Summary Toggle admin status for a user
// @Tags admin
// @Security BearerAuth
// @Param id path string true "User UUID"
// @Success 200 {object} UserWithAdminStatus
// @Router /admin/users/{id}/toggle-admin [post]
func (h *AdminHandler) ToggleAdmin(c *gin.Context) {
	adminUser := c.MustGet("user").(*models.User)
	userIDStr := c.Param("id")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	// Check current admin status
	isAdmin, _ := rbac.IsAdmin(user.ID)

	// Toggle admin status
	if isAdmin {
		if err := rbac.RevokeAdmin(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to revoke admin"})
			return
		}
		audit.LogAction(h.db, adminUser.ID, audit.ActionRevokeAdmin, "user:"+user.ID.String(), nil)
	} else {
		if err := rbac.MakeAdmin(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to make admin"})
			return
		}
		audit.LogAction(h.db, adminUser.ID, audit.ActionMakeAdmin, "user:"+user.ID.String(), nil)
	}

	c.JSON(http.StatusOK, UserWithAdminStatus{
		User:    user,
		IsAdmin: !isAdmin,
	})
}

// DeleteUser godoc
// @Summary Delete a user (admin only)
// @Tags admin
// @Security BearerAuth
// @Param id path string true "User UUID"
// @Success 204
// @Router /admin/users/{id} [delete]
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	adminUser := c.MustGet("user").(*models.User)
	userIDStr := c.Param("id")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// Can't delete yourself
	if userID == adminUser.ID {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Cannot delete yourself"})
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	if err := h.db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete user"})
		return
	}

	// Audit log
	audit.LogAction(h.db, adminUser.ID, audit.ActionDeleteUser, "user:"+user.ID.String(), map[string]interface{}{
		"username": user.Username,
	})

	c.Status(http.StatusNoContent)
}

// ListRoles godoc
// @Summary List all roles
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Role
// @Router /admin/roles [get]
func (h *AdminHandler) ListRoles(c *gin.Context) {
	var roles []models.Role
	if err := h.db.Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch roles"})
		return
	}

	c.JSON(http.StatusOK, roles)
}

// GrantPermission godoc
// @Summary Grant environment access to a user
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param permission body GrantPermissionRequest true "Permission details"
// @Success 201 {object} models.Permission
// @Router /admin/permissions [post]
func (h *AdminHandler) GrantPermission(c *gin.Context) {
	adminUser := c.MustGet("user").(*models.User)

	var req GrantPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Verify user exists
	var user models.User
	if err := h.db.First(&user, "id = ?", req.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	// Verify environment exists
	var env models.Environment
	if err := h.db.First(&env, "id = ?", req.EnvironmentID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Verify role exists
	var role models.Role
	if err := h.db.First(&role, req.RoleID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Role not found"})
		return
	}

	// Create permission record
	permission := models.Permission{
		UserID:        req.UserID,
		EnvironmentID: req.EnvironmentID,
		RoleID:        req.RoleID,
	}

	if err := h.db.Create(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create permission"})
		return
	}

	// Grant in RBAC
	if err := rbac.GrantEnvironmentAccess(user.ID, env.ID, role.Name); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to grant RBAC permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, adminUser.ID, audit.ActionGrantPermission, "permission:"+string(rune(permission.ID)), map[string]interface{}{
		"user_id":        req.UserID,
		"environment_id": req.EnvironmentID,
		"role":           role.Name,
	})

	c.JSON(http.StatusCreated, permission)
}

// ListPermissions godoc
// @Summary List all permissions
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Permission
// @Router /admin/permissions [get]
func (h *AdminHandler) ListPermissions(c *gin.Context) {
	var permissions []models.Permission
	if err := h.db.Preload("User").Preload("Environment").Preload("Role").Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch permissions"})
		return
	}

	c.JSON(http.StatusOK, permissions)
}

// RevokePermission godoc
// @Summary Revoke a permission
// @Tags admin
// @Security BearerAuth
// @Param id path string true "Permission ID"
// @Success 204
// @Router /admin/permissions/{id} [delete]
func (h *AdminHandler) RevokePermission(c *gin.Context) {
	adminUser := c.MustGet("user").(*models.User)
	permissionID := c.Param("id")

	var permission models.Permission
	if err := h.db.Preload("User").Preload("Environment").First(&permission, permissionID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Permission not found"})
		return
	}

	// Revoke from RBAC
	if err := rbac.RevokeEnvironmentAccess(permission.UserID, permission.EnvironmentID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to revoke RBAC permission"})
		return
	}

	// Delete permission record
	if err := h.db.Delete(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, adminUser.ID, audit.ActionRevokePermission, "permission:"+permissionID, map[string]interface{}{
		"user_id":        permission.UserID,
		"environment_id": permission.EnvironmentID,
	})

	c.Status(http.StatusNoContent)
}

// ListAuditLogs godoc
// @Summary List audit logs
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param user_id query string false "Filter by user ID"
// @Param action query string false "Filter by action"
// @Success 200 {array} models.AuditLog
// @Router /admin/audit-logs [get]
func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	query := h.db.Preload("User").Order("timestamp DESC").Limit(100)

	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	if action := c.Query("action"); action != "" {
		query = query.Where("action = ?", action)
	}

	var logs []models.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch audit logs"})
		return
	}

	c.JSON(http.StatusOK, logs)
}

// GetDashboardStats godoc
// @Summary Get admin dashboard statistics
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {object} DashboardStatsResponse
// @Router /admin/dashboard/stats [get]
func (h *AdminHandler) GetDashboardStats(c *gin.Context) {
	// Get total disk usage
	var totalDiskUsage struct {
		TotalBytes int64
	}
	h.db.Model(&models.Environment{}).
		Select("COALESCE(SUM(size_bytes), 0) as total_bytes").
		Scan(&totalDiskUsage)

	// Format size
	totalSizeFormatted := formatBytes(totalDiskUsage.TotalBytes)

	stats := DashboardStatsResponse{
		TotalDiskUsageBytes:     totalDiskUsage.TotalBytes,
		TotalDiskUsageFormatted: totalSizeFormatted,
	}

	c.JSON(http.StatusOK, stats)
}

// Request types
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	IsAdmin  bool   `json:"is_admin"`
}

type GrantPermissionRequest struct {
	UserID        uuid.UUID `json:"user_id" binding:"required"`
	EnvironmentID uuid.UUID `json:"environment_id" binding:"required"`
	RoleID        uint      `json:"role_id" binding:"required"`
}

type UserWithAdminStatus struct {
	models.User
	IsAdmin bool `json:"is_admin"`
}

type DashboardStatsResponse struct {
	TotalDiskUsageBytes     int64  `json:"total_disk_usage_bytes"`
	TotalDiskUsageFormatted string `json:"total_disk_usage_formatted"`
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
