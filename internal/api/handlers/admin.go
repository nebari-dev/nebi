package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/nebari-dev/nebi/internal/models" // imported for swagger type resolution
	"github.com/nebari-dev/nebi/internal/service"
)

type AdminHandler struct {
	svc *service.AdminService
}

func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

// ListUsers godoc
// @Summary List all users (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} service.UserWithAdmin
// @Router /admin/users [get]
func (h *AdminHandler) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsers()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, users)
}

// GetUser godoc
// @Summary Get user by ID (admin only)
// @Tags admin
// @Security BearerAuth
// @Param id path string true "User UUID"
// @Success 200 {object} service.UserWithAdmin
// @Router /admin/users/{id} [get]
func (h *AdminHandler) GetUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	user, err := h.svc.GetUser(userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

// ListUserGroups godoc
// @Summary List the groups a user belongs to (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path string true "User UUID"
// @Success 200 {array} models.Group
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/users/{id}/groups [get]
func (h *AdminHandler) ListUserGroups(c *gin.Context) {
	uid, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}
	groups, err := h.svc.ListUserGroups(uid)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, groups)
}

// ListRoles godoc
// @Summary List all roles
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Role
// @Router /admin/roles [get]
func (h *AdminHandler) ListRoles(c *gin.Context) {
	roles, err := h.svc.ListRoles()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, roles)
}

// GrantPermission godoc
// @Summary Grant project access to a user
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param permission body GrantPermissionRequest true "Permission details"
// @Success 201 {object} models.Permission
// @Router /admin/permissions [post]
func (h *AdminHandler) GrantPermission(c *gin.Context) {
	var req GrantPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}

	perm, err := h.svc.GrantPermission(req.UserID, req.ProjectID, req.RoleID, getAdminUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, perm)
}

// ListPermissions godoc
// @Summary List all permissions
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Permission
// @Router /admin/permissions [get]
func (h *AdminHandler) ListPermissions(c *gin.Context) {
	permissions, err := h.svc.ListPermissions()
	if err != nil {
		handleServiceError(c, err)
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
	if err := h.svc.RevokePermission(c.Param("id"), getAdminUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
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
	logs, err := h.svc.ListAuditLogs(c.Query("user_id"), c.Query("action"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, logs)
}

// GetDashboardStats godoc
// @Summary Get admin dashboard statistics
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {object} service.DashboardStats
// @Router /admin/dashboard/stats [get]
func (h *AdminHandler) GetDashboardStats(c *gin.Context) {
	stats, err := h.svc.GetDashboardStats()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

// GetResourceMetrics godoc
// @Summary Get resource limit and job usage metrics
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {object} service.ResourceMetrics
// @Router /admin/resource-metrics [get]
func (h *AdminHandler) GetResourceMetrics(c *gin.Context) {
	metrics, err := h.svc.GetResourceMetrics()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// --- Request types ---

type GrantPermissionRequest struct {
	UserID    uuid.UUID `json:"user_id" binding:"required"`
	ProjectID uuid.UUID `json:"project_id" binding:"required"`
	RoleID    uint      `json:"role_id" binding:"required"`
}

// getAdminUserID reuses getUserID from helpers.go.
var getAdminUserID = getUserID
