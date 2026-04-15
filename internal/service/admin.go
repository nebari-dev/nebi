package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AdminService contains business logic for admin operations.
type AdminService struct {
	db *gorm.DB
}

// NewAdminService creates a new AdminService.
func NewAdminService(db *gorm.DB) *AdminService {
	return &AdminService{db: db}
}

// UserWithAdmin wraps a user with their admin status.
type UserWithAdmin struct {
	models.User
	IsAdmin bool `json:"is_admin"`
}

// CreateUserRequest holds parameters for creating a user.
type CreateUserRequest struct {
	Username string
	Email    string
	Password string
	IsAdmin  bool
}

// DashboardStats holds admin dashboard statistics.
type DashboardStats struct {
	TotalDiskUsageBytes     int64  `json:"total_disk_usage_bytes"`
	TotalDiskUsageFormatted string `json:"total_disk_usage_formatted"`
}

// ListUsers returns all users with their admin status.
func (s *AdminService) ListUsers() ([]UserWithAdmin, error) {
	var users []models.User
	if err := s.db.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("fetch users: %w", err)
	}

	adminUserIDs, err := rbac.GetAllAdminUserIDs()
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

// CreateUser creates a new user, optionally granting admin, and writes an audit log.
func (s *AdminService) CreateUser(req CreateUserRequest, adminUserID uuid.UUID) (*models.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if req.IsAdmin {
		if err := rbac.MakeAdmin(user.ID); err != nil {
			return nil, fmt.Errorf("grant admin: %w", err)
		}
	}

	audit.LogAction(s.db, adminUserID, audit.ActionCreateUser, "user:"+user.ID.String(), map[string]any{
		"username": user.Username,
		"email":    user.Email,
		"is_admin": req.IsAdmin,
	})

	return &user, nil
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

	isAdmin, _ := rbac.IsAdmin(user.ID)
	return &UserWithAdmin{User: user, IsAdmin: isAdmin}, nil
}

// ToggleAdmin toggles admin status for a user and writes an audit log.
func (s *AdminService) ToggleAdmin(userID uuid.UUID, adminUserID uuid.UUID) (*UserWithAdmin, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	isAdmin, _ := rbac.IsAdmin(user.ID)

	if isAdmin {
		if err := rbac.RevokeAdmin(user.ID); err != nil {
			return nil, fmt.Errorf("revoke admin: %w", err)
		}
		audit.LogAction(s.db, adminUserID, audit.ActionRevokeAdmin, "user:"+user.ID.String(), nil)
	} else {
		if err := rbac.MakeAdmin(user.ID); err != nil {
			return nil, fmt.Errorf("make admin: %w", err)
		}
		audit.LogAction(s.db, adminUserID, audit.ActionMakeAdmin, "user:"+user.ID.String(), nil)
	}

	return &UserWithAdmin{User: user, IsAdmin: !isAdmin}, nil
}

// DeleteUser deletes a user and writes an audit log. Cannot delete self.
func (s *AdminService) DeleteUser(userID uuid.UUID, adminUserID uuid.UUID) error {
	if userID == adminUserID {
		return &ValidationError{Message: "Cannot delete yourself"}
	}

	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	if err := s.db.Delete(&user).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	audit.LogAction(s.db, adminUserID, audit.ActionDeleteUser, "user:"+user.ID.String(), map[string]any{
		"username": user.Username,
	})

	return nil
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
func (s *AdminService) GrantPermission(userID, workspaceID uuid.UUID, roleID uint, adminUserID uuid.UUID) (*models.Permission, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ValidationError{Message: "User not found"}
		}
		return nil, err
	}

	var ws models.Workspace
	if err := s.db.First(&ws, "id = ?", workspaceID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ValidationError{Message: "Workspace not found"}
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

	permission := models.Permission{
		UserID:      userID,
		WorkspaceID: workspaceID,
		RoleID:      roleID,
	}
	if err := s.db.Create(&permission).Error; err != nil {
		return nil, fmt.Errorf("create permission: %w", err)
	}

	if err := rbac.GrantWorkspaceAccess(user.ID, ws.ID, role.Name); err != nil {
		return nil, fmt.Errorf("grant RBAC permission: %w", err)
	}

	audit.LogAction(s.db, adminUserID, audit.ActionGrantPermission, fmt.Sprintf("permission:%d", permission.ID), map[string]any{
		"user_id":      userID,
		"workspace_id": workspaceID,
		"role":         role.Name,
	})

	return &permission, nil
}

// ListPermissions returns all permissions with preloaded relations.
func (s *AdminService) ListPermissions() ([]models.Permission, error) {
	var permissions []models.Permission
	if err := s.db.Preload("User").Preload("Workspace").Preload("Role").Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("fetch permissions: %w", err)
	}
	return permissions, nil
}

// RevokePermission revokes a permission by ID and removes RBAC access.
func (s *AdminService) RevokePermission(permissionID string, adminUserID uuid.UUID) error {
	var permission models.Permission
	if err := s.db.Preload("User").Preload("Workspace").First(&permission, permissionID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	if err := rbac.RevokeWorkspaceAccess(permission.UserID, permission.WorkspaceID); err != nil {
		return fmt.Errorf("revoke RBAC permission: %w", err)
	}

	if err := s.db.Delete(&permission).Error; err != nil {
		return fmt.Errorf("delete permission: %w", err)
	}

	audit.LogAction(s.db, adminUserID, audit.ActionRevokePermission, "permission:"+permissionID, map[string]any{
		"user_id":      permission.UserID,
		"workspace_id": permission.WorkspaceID,
	})

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
	if err := s.db.Model(&models.Workspace{}).
		Select("COALESCE(SUM(size_bytes), 0) as total_bytes").
		Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("fetch dashboard stats: %w", err)
	}

	return &DashboardStats{
		TotalDiskUsageBytes:     result.TotalBytes,
		TotalDiskUsageFormatted: utils.FormatBytes(result.TotalBytes),
	}, nil
}
