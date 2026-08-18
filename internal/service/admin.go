package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/limits"
	resourcemetrics "github.com/nebari-dev/nebi/internal/metrics"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/utils"
	"golang.org/x/crypto/bcrypt"
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

type ResourceMetrics struct {
	Limits                limits.Limits                          `json:"limits"`
	ActiveJobsGlobal      int64                                  `json:"active_jobs_global"`
	ActiveJobsByUser      []UserActiveJobUsage                   `json:"active_jobs_by_user"`
	ActiveJobsByWorkspace []WorkspaceActiveJobUsage              `json:"active_jobs_by_workspace"`
	QuotaRejections       resourcemetrics.QuotaRejectionSnapshot `json:"quota_rejections"`
	JobTimeoutsTotal      int64                                  `json:"job_timeouts_total"`
}

type UserActiveJobUsage struct {
	UserID     uuid.UUID `json:"user_id"`
	ActiveJobs int64     `json:"active_jobs"`
}

type WorkspaceActiveJobUsage struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ActiveJobs  int64     `json:"active_jobs"`
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
		if err := s.rbac.MakeAdmin(user.ID); err != nil {
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

	isAdmin, _ := s.rbac.IsAdmin(user.ID)
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

	isAdmin, _ := s.rbac.IsAdmin(user.ID)

	if isAdmin {
		if err := s.rbac.RevokeAdmin(user.ID); err != nil {
			return nil, fmt.Errorf("revoke admin: %w", err)
		}
		audit.LogAction(s.db, adminUserID, audit.ActionRevokeAdmin, "user:"+user.ID.String(), nil)
	} else {
		if err := s.rbac.MakeAdmin(user.ID); err != nil {
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

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("user_id = ?", user.ID).Delete(&models.FederatedIdentity{}).Error; err != nil {
			return fmt.Errorf("delete federated identities: %w", err)
		}
		// Reviews are collision-specific to this user. Once the target user is
		// deleted, remove both pending and rejected reviews so future sign-ins
		// for the same issuer/subject are evaluated against the remaining
		// active accounts.
		if err := tx.Unscoped().Where("user_id = ?", user.ID).Delete(&models.FederatedIdentityReview{}).Error; err != nil {
			return fmt.Errorf("delete federated identity reviews: %w", err)
		}
		if err := tx.Delete(&user).Error; err != nil {
			return fmt.Errorf("delete user: %w", err)
		}

		audit.LogAction(tx, adminUserID, audit.ActionDeleteUser, "user:"+user.ID.String(), map[string]any{
			"username": user.Username,
		})
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// ListUserGroups returns every group the given user belongs to (native + OIDC).
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

	var permission models.Permission
	err := s.db.Transaction(func(tx *gorm.DB) error {
		permission = models.Permission{
			UserID:      userID,
			WorkspaceID: workspaceID,
			RoleID:      roleID,
		}
		if err := tx.Create(&permission).Error; err != nil {
			return fmt.Errorf("create permission: %w", err)
		}

		audit.LogAction(tx, adminUserID, audit.ActionGrantPermission, fmt.Sprintf("permission:%d", permission.ID), map[string]any{
			"user_id":      userID,
			"workspace_id": workspaceID,
			"role":         role.Name,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.rbac.GrantWorkspaceAccess(user.ID, ws.ID, role.Name); err != nil {
		return nil, fmt.Errorf("grant RBAC permission: %w", err)
	}

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
	if err := s.db.Preload("User").Preload("Workspace").First(&permission, "id = ?", permissionID).Error; err != nil {
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
			"user_id":      permission.UserID,
			"workspace_id": permission.WorkspaceID,
		})

		return nil
	})
	if err != nil {
		return err
	}

	if err := s.rbac.RevokeWorkspaceAccess(permission.UserID, permission.WorkspaceID); err != nil {
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

// ListFederatedIdentityReviews returns federated identity review decisions.
func (s *AdminService) ListFederatedIdentityReviews(statusFilter string) ([]models.FederatedIdentityReview, error) {
	if statusFilter == "" {
		statusFilter = models.FederatedIdentityReviewStatusPending
	}

	query := s.db.
		Preload("User").
		Preload("CollisionUsernameUser").
		Preload("CollisionEmailUser").
		Order("created_at DESC").
		Limit(100)
	switch statusFilter {
	case models.FederatedIdentityReviewStatusPending:
		query = query.Where("status = ? OR status = ''", models.FederatedIdentityReviewStatusPending)
	case models.FederatedIdentityReviewStatusRejected:
		query = query.Where("status = ?", models.FederatedIdentityReviewStatusRejected)
	case "all":
	default:
		return nil, &ValidationError{Message: "status must be 'pending', 'rejected', or 'all'"}
	}

	var reviews []models.FederatedIdentityReview
	if err := query.Find(&reviews).Error; err != nil {
		return nil, fmt.Errorf("fetch federated identity reviews: %w", err)
	}
	return reviews, nil
}

// ApproveFederatedIdentityReview deliberately links a reviewed external
// identity to the colliding local user.
func (s *AdminService) ApproveFederatedIdentityReview(reviewID uuid.UUID, adminUserID uuid.UUID) (*models.FederatedIdentity, error) {
	var identity models.FederatedIdentity
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var review models.FederatedIdentityReview
		if err := tx.Preload("User").First(&review, "id = ?", reviewID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if review.User.ID != review.UserID {
			return &ConflictError{
				Message: fmt.Sprintf("federated identity review is linked to deleted user %s; discard it and ask the user to sign in again", review.UserID),
			}
		}
		if !review.IsPending() {
			return &ConflictError{Message: "federated identity review was rejected"}
		}
		if review.HasAmbiguousCollision() {
			return &ConflictError{Message: "federated identity review has username and email collisions with different users; discard it after resolving the conflicting accounts"}
		}

		var existing models.FederatedIdentity
		err := tx.Where("issuer = ? AND subject = ?", review.Issuer, review.Subject).First(&existing).Error
		if err == nil {
			return &ConflictError{Message: "federated identity is already approved"}
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		identity = models.FederatedIdentity{
			UserID:        review.UserID,
			Issuer:        review.Issuer,
			Subject:       review.Subject,
			Username:      review.Username,
			Email:         review.Email,
			EmailVerified: review.EmailVerified,
			Name:          review.Name,
			AvatarURL:     review.AvatarURL,
		}
		if err := tx.Create(&identity).Error; err != nil {
			return fmt.Errorf("approve federated identity: %w", err)
		}
		if err := tx.Unscoped().Delete(&review).Error; err != nil {
			return fmt.Errorf("delete federated identity review: %w", err)
		}

		audit.LogAction(tx, adminUserID, audit.ActionApproveFederatedIdentity, audit.ResourceFederatedIdentityReview+":"+review.ID.String(), map[string]any{
			"identity_id":     identity.ID,
			"user_id":         review.UserID,
			"review_id":       review.ID,
			"issuer":          review.Issuer,
			"subject":         review.Subject,
			"collision_field": review.CollisionField,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return &identity, nil
}

// RejectFederatedIdentityReview marks a pending external identity link request
// as rejected without linking it to the colliding local user.
func (s *AdminService) RejectFederatedIdentityReview(reviewID uuid.UUID, adminUserID uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var review models.FederatedIdentityReview
		if err := tx.First(&review, "id = ?", reviewID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if !review.IsPending() {
			return &ConflictError{Message: "federated identity review was already rejected"}
		}

		now := time.Now().UTC()
		result := tx.Model(&models.FederatedIdentityReview{}).
			Where("id = ?", review.ID).
			Where("status = ? OR status = ''", models.FederatedIdentityReviewStatusPending).
			Updates(map[string]any{
				"status":      models.FederatedIdentityReviewStatusRejected,
				"reviewed_by": adminUserID,
				"reviewed_at": now,
			})
		if result.Error != nil {
			return fmt.Errorf("reject federated identity review: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			var current models.FederatedIdentityReview
			err := tx.Select("status").First(&current, "id = ?", review.ID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			return &ConflictError{Message: "federated identity review is no longer pending"}
		}

		audit.LogAction(tx, adminUserID, audit.ActionRejectFederatedIdentity, audit.ResourceFederatedIdentityReview+":"+review.ID.String(), map[string]any{
			"user_id":         review.UserID,
			"issuer":          review.Issuer,
			"subject":         review.Subject,
			"collision_field": review.CollisionField,
		})
		return nil
	})
}

// DiscardFederatedIdentityReview permanently deletes a pending or rejected
// review, allowing a future login for the same issuer and subject to create a
// fresh review if a collision still exists.
func (s *AdminService) DiscardFederatedIdentityReview(reviewID uuid.UUID, adminUserID uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var review models.FederatedIdentityReview
		if err := tx.First(&review, "id = ?", reviewID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Unscoped().Delete(&review).Error; err != nil {
			return fmt.Errorf("discard federated identity review: %w", err)
		}

		audit.LogAction(tx, adminUserID, audit.ActionDiscardFederatedIdentity, audit.ResourceFederatedIdentityReview+":"+review.ID.String(), map[string]any{
			"user_id":         review.UserID,
			"issuer":          review.Issuer,
			"subject":         review.Subject,
			"collision_field": review.CollisionField,
			"status":          review.Status,
		})
		return nil
	})
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

func (s *AdminService) GetResourceMetrics() (*ResourceMetrics, error) {
	var activeGlobal int64
	if err := s.db.Model(&models.Job{}).Where("status IN ?", activeJobStatuses).Count(&activeGlobal).Error; err != nil {
		return nil, fmt.Errorf("count active jobs: %w", err)
	}

	effectiveUserID := fmt.Sprintf("COALESCE(NULLIF(NULLIF(jobs.user_id, '%s'), ''), workspaces.owner_id)", uuid.Nil.String())
	var activeByUser []UserActiveJobUsage
	if err := s.db.Model(&models.Job{}).
		Select(effectiveUserID+" AS user_id, COUNT(*) AS active_jobs").
		Joins("LEFT JOIN workspaces ON workspaces.id = jobs.workspace_id").
		Where("jobs.status IN ?", activeJobStatuses).
		Where(effectiveUserID + " IS NOT NULL").
		Group(effectiveUserID).
		Scan(&activeByUser).Error; err != nil {
		return nil, fmt.Errorf("count active jobs by user: %w", err)
	}

	var activeByWorkspace []WorkspaceActiveJobUsage
	if err := s.db.Model(&models.Job{}).
		Select("workspace_id, COUNT(*) as active_jobs").
		Where("status IN ?", activeJobStatuses).
		Group("workspace_id").
		Scan(&activeByWorkspace).Error; err != nil {
		return nil, fmt.Errorf("count active jobs by workspace: %w", err)
	}

	snapshot, err := resourcemetrics.Snapshot(s.db)
	if err != nil {
		return nil, err
	}
	return &ResourceMetrics{
		Limits:                s.limits,
		ActiveJobsGlobal:      activeGlobal,
		ActiveJobsByUser:      activeByUser,
		ActiveJobsByWorkspace: activeByWorkspace,
		QuotaRejections:       snapshot.QuotaRejections,
		JobTimeoutsTotal:      snapshot.JobTimeouts,
	}, nil
}

// GrantGroupAdmin promotes a group to admin. Every current and future member
// of the group becomes an effective admin via Casbin g + p rules.
func (s *AdminService) GrantGroupAdmin(groupID uuid.UUID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if g.Source == models.GroupSourceOIDC {
		return &ConflictError{Message: "cannot grant admin to an OIDC-synced group; its membership is controlled by the identity provider"}
	}
	if err := s.rbac.MakeGroupAdmin(groupID); err != nil {
		return fmt.Errorf("make group admin: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionGrantGroupAdmin, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
		"group_name": g.Name,
	})
	return nil
}

// RevokeGroupAdmin removes the group's admin grant.
func (s *AdminService) RevokeGroupAdmin(groupID uuid.UUID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := s.rbac.RevokeGroupAdmin(groupID); err != nil {
		return fmt.Errorf("revoke group admin: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionRevokeGroupAdmin, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
		"group_name": g.Name,
	})
	return nil
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
