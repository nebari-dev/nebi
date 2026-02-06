package rbac

import (
	_ "embed"
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

//go:embed model.conf
var modelConf string

var enforcer *casbin.Enforcer

// InitEnforcer initializes the Casbin enforcer
func InitEnforcer(db *gorm.DB, logger *slog.Logger) error {
	adapter, err := gormadapter.NewAdapterByDB(db)
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

// CanReadWorkspace checks if user can read a workspace
func CanReadWorkspace(userID uuid.UUID, wsID uuid.UUID) (bool, error) {
	return enforcer.Enforce(userID.String(), fmt.Sprintf("ws:%s", wsID.String()), "read")
}

// CanWriteWorkspace checks if user can write to a workspace
func CanWriteWorkspace(userID uuid.UUID, wsID uuid.UUID) (bool, error) {
	return enforcer.Enforce(userID.String(), fmt.Sprintf("ws:%s", wsID.String()), "write")
}

// IsAdmin checks if user has admin privileges
func IsAdmin(userID uuid.UUID) (bool, error) {
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

	_, err := enforcer.AddPolicy(userID.String(), fmt.Sprintf("ws:%s", wsID.String()), action)
	if err != nil {
		return err
	}

	return enforcer.SavePolicy()
}

// RevokeWorkspaceAccess revokes access to a workspace
func RevokeWorkspaceAccess(userID uuid.UUID, wsID uuid.UUID) error {
	obj := fmt.Sprintf("ws:%s", wsID.String())

	// Remove both read and write permissions
	enforcer.RemovePolicy(userID.String(), obj, "read")
	enforcer.RemovePolicy(userID.String(), obj, "write")

	return enforcer.SavePolicy()
}

// MakeAdmin grants admin privileges to a user
func MakeAdmin(userID uuid.UUID) error {
	_, err := enforcer.AddPolicy(userID.String(), "admin", "admin")
	if err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// RevokeAdmin removes admin privileges from a user
func RevokeAdmin(userID uuid.UUID) error {
	_, err := enforcer.RemovePolicy(userID.String(), "admin", "admin")
	if err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// GetAllAdminUserIDs returns a set of all user IDs that have admin privileges
func GetAllAdminUserIDs() (map[uuid.UUID]bool, error) {
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
