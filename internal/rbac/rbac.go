package rbac

import (
	_ "embed"
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

//go:embed model.conf
var modelConf string

var enforcer *casbin.Enforcer

// InitEnforcer initializes the Casbin enforcer
func InitEnforcer(db *gorm.DB, logger *slog.Logger) error {
	adapter, err := newGormAdapter(db)
	if err != nil {
		return fmt.Errorf("failed to create casbin adapter: %w", err)
	}

	// Load model from embedded string
	m, err := model.NewModelFromString(modelConf)
	if err != nil {
		return fmt.Errorf("failed to parse casbin model: %w", err)
	}

	e, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return fmt.Errorf("failed to create casbin enforcer: %w", err)
	}

	// Load policies from database
	if err := e.LoadPolicy(); err != nil {
		return fmt.Errorf("failed to load policies: %w", err)
	}

	enforcer = e
	logger.Info("RBAC enforcer initialized")
	return nil
}

// GetEnforcer returns the global enforcer instance
func GetEnforcer() *casbin.Enforcer {
	return enforcer
}

// CanReadWorkspace checks if user can read a workspace.
// Local mode (cfg.IsLocalMode()) skips InitEnforcer because the
// middleware bypasses RBAC anyway, so the enforcer global stays nil.
// The data-layer functions defend against that explicitly: nothing to
// enforce against = treat as allowed. Without this guard
// WorkspaceService.Create nil-derefs through GrantWorkspaceAccess on
// a fresh local-mode boot.
func CanReadWorkspace(userID uuid.UUID, wsID uuid.UUID) (bool, error) {
	if enforcer == nil {
		return true, nil
	}
	return enforcer.Enforce(userID.String(), fmt.Sprintf("ws:%s", wsID.String()), "read")
}

// CanWriteWorkspace checks if user can write to a workspace
func CanWriteWorkspace(userID uuid.UUID, wsID uuid.UUID) (bool, error) {
	if enforcer == nil {
		return true, nil
	}
	return enforcer.Enforce(userID.String(), fmt.Sprintf("ws:%s", wsID.String()), "write")
}

// IsAdmin checks if user has admin privileges
func IsAdmin(userID uuid.UUID) (bool, error) {
	if enforcer == nil {
		return true, nil
	}
	return enforcer.Enforce(userID.String(), "admin", "admin")
}

// GrantWorkspaceAccess grants access to a workspace
func GrantWorkspaceAccess(userID uuid.UUID, wsID uuid.UUID, role string) error {
	var action string
	switch role {
	case "owner", "editor":
		action = "write"
	case "viewer":
		action = "read"
	default:
		return fmt.Errorf("invalid role: %s", role)
	}

	if enforcer == nil {
		return nil
	}

	_, err := enforcer.AddPolicy(userID.String(), fmt.Sprintf("ws:%s", wsID.String()), action)
	return err
}

// RevokeWorkspaceAccess revokes access to a workspace
func RevokeWorkspaceAccess(userID uuid.UUID, wsID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}

	obj := fmt.Sprintf("ws:%s", wsID.String())

	if _, err := enforcer.RemovePolicy(userID.String(), obj, "read"); err != nil {
		return err
	}
	_, err := enforcer.RemovePolicy(userID.String(), obj, "write")
	return err
}

// MakeAdmin grants admin privileges to a user
func MakeAdmin(userID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.AddPolicy(userID.String(), "admin", "admin")
	return err
}

// RevokeAdmin removes admin privileges from a user
func RevokeAdmin(userID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.RemovePolicy(userID.String(), "admin", "admin")
	return err
}

// GetAllAdminUserIDs returns a set of all user IDs that have admin privileges
func GetAllAdminUserIDs() (map[uuid.UUID]bool, error) {
	if enforcer == nil {
		return map[uuid.UUID]bool{}, nil
	}
	// Get all policies where object="admin" and action="admin" in ONE call
	policies, err := enforcer.GetFilteredPolicy(1, "admin", "admin")
	if err != nil {
		return nil, err
	}

	adminUserIDs := make(map[uuid.UUID]bool, len(policies))
	for _, policy := range policies {
		if len(policy) >= 1 {
			if userID, err := uuid.Parse(policy[0]); err == nil {
				adminUserIDs[userID] = true
			}
		}
	}

	return adminUserIDs, nil
}

// GetUserWorkspaces returns all workspace IDs that a user has access to
func GetUserWorkspaces(userID uuid.UUID) ([]uuid.UUID, error) {
	if enforcer == nil {
		return []uuid.UUID{}, nil
	}
	policies, err := enforcer.GetFilteredPolicy(0, userID.String())
	if err != nil {
		return nil, err
	}

	wsIDs := make([]uuid.UUID, 0)
	for _, policy := range policies {
		if len(policy) >= 2 && len(policy[1]) > 3 && policy[1][:3] == "ws:" {
			wsIDStr := policy[1][3:] // Remove "ws:" prefix
			if wsID, err := uuid.Parse(wsIDStr); err == nil {
				wsIDs = append(wsIDs, wsID)
			}
		}
	}

	return wsIDs, nil
}

// AddUserToGroup creates a grouping rule g(userID, groupID).
func AddUserToGroup(userID, groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.AddGroupingPolicy(userID.String(), groupID.String())
	return err
}

// RemoveUserFromGroup removes the grouping rule g(userID, groupID).
func RemoveUserFromGroup(userID, groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.RemoveGroupingPolicy(userID.String(), groupID.String())
	return err
}

// GetUserGroups returns the group IDs the user belongs to via Casbin grouping rules.
func GetUserGroups(userID uuid.UUID) ([]uuid.UUID, error) {
	if enforcer == nil {
		return nil, nil
	}
	roles, err := enforcer.GetRolesForUser(userID.String())
	if err != nil {
		return nil, err
	}
	groups := make([]uuid.UUID, 0, len(roles))
	for _, r := range roles {
		if id, err := uuid.Parse(r); err == nil {
			groups = append(groups, id)
		}
	}
	return groups, nil
}

// GrantGroupWorkspaceAccess grants a group access to a workspace.
func GrantGroupWorkspaceAccess(groupID, wsID uuid.UUID, role string) error {
	var action string
	switch role {
	case "owner", "editor":
		action = "write"
	case "viewer":
		action = "read"
	default:
		return fmt.Errorf("invalid role: %s", role)
	}
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.AddPolicy(groupID.String(), fmt.Sprintf("ws:%s", wsID.String()), action)
	return err
}

// RevokeGroupWorkspaceAccess revokes a group's access to a workspace.
func RevokeGroupWorkspaceAccess(groupID, wsID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	obj := fmt.Sprintf("ws:%s", wsID.String())
	if _, err := enforcer.RemovePolicy(groupID.String(), obj, "read"); err != nil {
		return err
	}
	_, err := enforcer.RemovePolicy(groupID.String(), obj, "write")
	return err
}

// GrantGroupRegistryAccess grants a group access to a registry (read or write).
func GrantGroupRegistryAccess(groupID, regID uuid.UUID, action string) error {
	if action != "read" && action != "write" {
		return fmt.Errorf("invalid registry action: %s", action)
	}
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.AddPolicy(groupID.String(), fmt.Sprintf("reg:%s", regID.String()), action)
	return err
}

// RevokeGroupRegistryAccess revokes a group's access to a registry.
func RevokeGroupRegistryAccess(groupID, regID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	obj := fmt.Sprintf("reg:%s", regID.String())
	if _, err := enforcer.RemovePolicy(groupID.String(), obj, "read"); err != nil {
		return err
	}
	_, err := enforcer.RemovePolicy(groupID.String(), obj, "write")
	return err
}

// CanReadRegistry checks if a subject can read a registry (transitive via groups).
func CanReadRegistry(userID, regID uuid.UUID) (bool, error) {
	if enforcer == nil {
		return true, nil
	}
	return enforcer.Enforce(userID.String(), fmt.Sprintf("reg:%s", regID.String()), "read")
}

// CanWriteRegistry checks if a subject can write to a registry (transitive via groups).
func CanWriteRegistry(userID, regID uuid.UUID) (bool, error) {
	if enforcer == nil {
		return true, nil
	}
	return enforcer.Enforce(userID.String(), fmt.Sprintf("reg:%s", regID.String()), "write")
}

// MakeGroupAdmin grants admin privileges to every member of a group.
func MakeGroupAdmin(groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.AddPolicy(groupID.String(), "admin", "admin")
	return err
}

// RevokeGroupAdmin removes group-level admin privilege.
func RevokeGroupAdmin(groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	_, err := enforcer.RemovePolicy(groupID.String(), "admin", "admin")
	return err
}

// RemoveAllGroupPolicies removes every Casbin rule that involves a group:
//   - All `p` policies where the group is the subject (workspace, registry, admin grants).
//   - All `g` grouping rules where the group is the role (memberships).
//
// Casbin doesn't honor GORM soft-delete, so this is a hard remove.
func RemoveAllGroupPolicies(groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.RemoveFilteredPolicy(0, groupID.String()); err != nil {
		return err
	}
	_, err := enforcer.RemoveFilteredGroupingPolicy(1, groupID.String())
	return err
}
