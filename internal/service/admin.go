package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/limits"
	resourcemetrics "github.com/nebari-dev/nebi/internal/metrics"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/utils"
	"gorm.io/gorm"
)

// AdminService contains business logic for admin operations.
type AdminService struct {
	db     *gorm.DB
	rbac   rbac.Provider
	limits limits.Limits
}

// NewAdminService creates a new AdminService.
func NewAdminService(db *gorm.DB, rbacProvider rbac.Provider, limitCfg limits.Limits) *AdminService {
	return &AdminService{db: db, rbac: rbacProvider, limits: limitCfg}
}

// UserWithAdmin wraps a user with their admin status.
type UserWithAdmin struct {
	models.User
	IsAdmin bool `json:"is_admin"`
}

// DashboardStats holds admin dashboard statistics.
type DashboardStats struct {
	TotalDiskUsageBytes     int64  `json:"total_disk_usage_bytes"`
	TotalDiskUsageFormatted string `json:"total_disk_usage_formatted"`
}

type ResourceMetrics struct {
	Limits              limits.Limits                          `json:"limits"`
	ActiveJobsGlobal    int64                                  `json:"active_jobs_global"`
	ActiveJobsByUser    []UserActiveJobUsage                   `json:"active_jobs_by_user"`
	ActiveJobsByProject []ProjectActiveJobUsage                `json:"active_jobs_by_project"`
	QuotaRejections     resourcemetrics.QuotaRejectionSnapshot `json:"quota_rejections"`
	JobTimeoutsTotal    int64                                  `json:"job_timeouts_total"`
}

type UserActiveJobUsage struct {
	UserID     uuid.UUID `json:"user_id"`
	ActiveJobs int64     `json:"active_jobs"`
}

type ProjectActiveJobUsage struct {
	ProjectID  uuid.UUID `json:"project_id"`
	ActiveJobs int64     `json:"active_jobs"`
}

// ListUsers returns all users with their admin status.
func (s *AdminService) ListUsers() ([]UserWithAdmin, error) {
	var users []models.User
	if err := s.db.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("fetch users: %w", err)
	}

	adminUserIDs, err := s.rbac.GetAllAdminUserIDs()
	if err != nil {
		return nil, fmt.Errorf("check admin status: %w", err)
	}

	result := make([]UserWithAdmin, len(users))
	for i, user := range users {
		result[i] = UserWithAdmin{
			User:    user,
			IsAdmin: adminUserIDs[user.ID],
		}
	}
	return result, nil
}

// GetUser returns a user by ID with admin status.
func (s *AdminService) GetUser(userID uuid.UUID) (*UserWithAdmin, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	isAdmin, _ := s.rbac.IsAdmin(user.ID)
	return &UserWithAdmin{User: user, IsAdmin: isAdmin}, nil
}

// ListUserGroups returns every group the given user belongs to.
func (s *AdminService) ListUserGroups(userID uuid.UUID) ([]models.Group, error) {
	var groups []models.Group
	err := s.db.
		Joins("JOIN group_members gm ON gm.group_id = groups.id").
		Where("gm.user_id = ?", userID).
		Find(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	return groups, nil
}

// ListRoles returns all roles.
func (s *AdminService) ListRoles() ([]models.Role, error) {
	var roles []models.Role
	if err := s.db.Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("fetch roles: %w", err)
	}
	return roles, nil
}

// GrantPermission creates a permission record and grants RBAC access.
func (s *AdminService) GrantPermission(userID, projectID uuid.UUID, roleID uint, adminUserID uuid.UUID) (*models.Permission, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ValidationError{Message: "User not found"}
		}
		return nil, err
	}

	var project models.Project
	if err := s.db.First(&project, "id = ?", projectID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ValidationError{Message: "Project not found"}
		}
		return nil, err
	}

	var role models.Role
	if err := s.db.First(&role, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ValidationError{Message: "Role not found"}
		}
		return nil, err
	}

	var permission models.Permission
	err := s.db.Transaction(func(tx *gorm.DB) error {
		permission = models.Permission{
			UserID:    userID,
			ProjectID: projectID,
			RoleID:    roleID,
		}
		if err := tx.Create(&permission).Error; err != nil {
			return fmt.Errorf("create permission: %w", err)
		}

		audit.LogAction(tx, adminUserID, audit.ActionGrantPermission, fmt.Sprintf("permission:%d", permission.ID), map[string]any{
			"user_id":    userID,
			"project_id": projectID,
			"role":       role.Name,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.rbac.GrantProjectAccess(user.ID, project.ID, role.Name); err != nil {
		return nil, fmt.Errorf("grant RBAC permission: %w", err)
	}

	return &permission, nil
}

// ListPermissions returns all permissions with preloaded relations.
func (s *AdminService) ListPermissions() ([]models.Permission, error) {
	var permissions []models.Permission
	if err := s.db.Preload("User").Preload("Project").Preload("Role").Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("fetch permissions: %w", err)
	}
	return permissions, nil
}

// RevokePermission revokes a permission by ID and removes RBAC access.
func (s *AdminService) RevokePermission(permissionID string, adminUserID uuid.UUID) error {
	var permission models.Permission
	if err := s.db.Preload("User").Preload("Project").First(&permission, "id = ?", permissionID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&permission).Error; err != nil {
			return fmt.Errorf("delete permission: %w", err)
		}

		audit.LogAction(tx, adminUserID, audit.ActionRevokePermission, "permission:"+permissionID, map[string]any{
			"user_id":    permission.UserID,
			"project_id": permission.ProjectID,
		})

		return nil
	})
	if err != nil {
		return err
	}

	if err := s.rbac.RevokeProjectAccess(permission.UserID, permission.ProjectID); err != nil {
		return fmt.Errorf("revoke RBAC permission: %w", err)
	}

	return nil
}

// ListAuditLogs returns audit logs with optional filters.
func (s *AdminService) ListAuditLogs(userIDFilter, actionFilter string) ([]models.AuditLog, error) {
	query := s.db.Preload("User").Order("timestamp DESC").Limit(100)

	if userIDFilter != "" {
		query = query.Where("user_id = ?", userIDFilter)
	}
	if actionFilter != "" {
		query = query.Where("action = ?", actionFilter)
	}

	var logs []models.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("fetch audit logs: %w", err)
	}
	return logs, nil
}

// GetDashboardStats returns admin dashboard statistics.
func (s *AdminService) GetDashboardStats() (*DashboardStats, error) {
	var result struct {
		TotalBytes int64
	}
	if err := s.db.Model(&models.Project{}).
		Select("COALESCE(SUM(size_bytes), 0) as total_bytes").
		Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("fetch dashboard stats: %w", err)
	}

	return &DashboardStats{
		TotalDiskUsageBytes:     result.TotalBytes,
		TotalDiskUsageFormatted: utils.FormatBytes(result.TotalBytes),
	}, nil
}

func (s *AdminService) GetResourceMetrics() (*ResourceMetrics, error) {
	var activeGlobal int64
	if err := s.db.Model(&models.Job{}).Where("status IN ?", activeJobStatuses).Count(&activeGlobal).Error; err != nil {
		return nil, fmt.Errorf("count active jobs: %w", err)
	}

	effectiveUserID := fmt.Sprintf("COALESCE(NULLIF(NULLIF(jobs.user_id, '%s'), ''), projects.owner_id)", uuid.Nil.String())
	var activeByUser []UserActiveJobUsage
	if err := s.db.Model(&models.Job{}).
		Select(effectiveUserID+" AS user_id, COUNT(*) AS active_jobs").
		Joins("LEFT JOIN projects ON projects.id = jobs.project_id").
		Where("jobs.status IN ?", activeJobStatuses).
		Where(effectiveUserID + " IS NOT NULL").
		Group(effectiveUserID).
		Scan(&activeByUser).Error; err != nil {
		return nil, fmt.Errorf("count active jobs by user: %w", err)
	}

	var activeByProject []ProjectActiveJobUsage
	if err := s.db.Model(&models.Job{}).
		Select("project_id, COUNT(*) as active_jobs").
		Where("status IN ?", activeJobStatuses).
		Group("project_id").
		Scan(&activeByProject).Error; err != nil {
		return nil, fmt.Errorf("count active jobs by project: %w", err)
	}

	snapshot, err := resourcemetrics.Snapshot(s.db)
	if err != nil {
		return nil, err
	}
	return &ResourceMetrics{
		Limits:              s.limits,
		ActiveJobsGlobal:    activeGlobal,
		ActiveJobsByUser:    activeByUser,
		ActiveJobsByProject: activeByProject,
		QuotaRejections:     snapshot.QuotaRejections,
		JobTimeoutsTotal:    snapshot.JobTimeouts,
	}, nil
}

// GrantRegistryToGroup grants a group access to a registry (read or write).
func (s *AdminService) GrantRegistryToGroup(regID, groupID uuid.UUID, action string, actorID uuid.UUID) error {
	if action != "read" && action != "write" {
		return &ValidationError{Message: "action must be 'read' or 'write'"}
	}
	var reg models.OCIRegistry
	if err := s.db.First(&reg, "id = ?", regID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ValidationError{Message: "Registry not found"}
		}
		return err
	}
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ValidationError{Message: "Group not found"}
		}
		return err
	}
	if err := s.rbac.GrantGroupRegistryAccess(groupID, regID, action); err != nil {
		return fmt.Errorf("grant registry: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionGrantGroupPerm, fmt.Sprintf("reg:%s", regID), map[string]interface{}{
		"group_id": groupID,
		"action":   action,
	})
	return nil
}

// RevokeRegistryFromGroup removes a group's access to a registry.
func (s *AdminService) RevokeRegistryFromGroup(regID, groupID uuid.UUID, actorID uuid.UUID) error {
	if err := s.rbac.RevokeGroupRegistryAccess(groupID, regID); err != nil {
		return fmt.Errorf("revoke registry: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionRevokeGroupPerm, fmt.Sprintf("reg:%s", regID), map[string]interface{}{
		"group_id": groupID,
	})
	return nil
}
