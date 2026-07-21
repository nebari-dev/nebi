package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func hasRegistryAccess(provider rbac.Provider, isLocal bool, userID, regID uuid.UUID, action string) (bool, error) {
	if isLocal {
		return true, nil
	}
	if provider == nil {
		return false, fmt.Errorf("registry RBAC provider not configured")
	}

	switch action {
	case "read":
		return provider.CanReadRegistry(userID, regID)
	case "write":
		return provider.CanWriteRegistry(userID, regID)
	default:
		return false, fmt.Errorf("invalid registry action: %s", action)
	}
}

func ensureRegistryAccess(db *gorm.DB, provider rbac.Provider, isLocal bool, userID, regID uuid.UUID, action string) error {
	hasAccess, err := hasRegistryAccess(provider, isLocal, userID, regID, action)
	if err != nil {
		return fmt.Errorf("check registry %s access: %w", action, err)
	}
	if hasAccess {
		return nil
	}

	_ = audit.LogAction(db, userID, audit.ActionRegistryAccessDenied, fmt.Sprintf("reg:%s", regID), map[string]any{
		"action": action,
	})
	return &ForbiddenError{Message: "Registry access denied"}
}
