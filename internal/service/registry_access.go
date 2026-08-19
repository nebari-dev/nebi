package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func hasRegistryAccess(provider rbac.Provider, isLocal bool, userID uuid.UUID, registry models.OCIRegistry, action string) (bool, error) {
	if err := validateRegistryAction(action); err != nil {
		return false, err
	}
	if isLocal {
		return true, nil
	}
	if !registry.Restricted {
		return true, nil
	}
	if provider == nil {
		return false, fmt.Errorf("registry RBAC provider not configured")
	}

	switch action {
	case "read":
		return provider.CanReadRegistry(userID, registry.ID)
	case "write":
		return provider.CanWriteRegistry(userID, registry.ID)
	}
	return false, nil
}

func ensureRegistryAccess(db *gorm.DB, provider rbac.Provider, isLocal bool, userID, regID uuid.UUID, action string) error {
	var registry models.OCIRegistry
	if err := db.Select("id", "restricted").Where("id = ?", regID).First(&registry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	hasAccess, err := hasRegistryAccess(provider, isLocal, userID, registry, action)
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

func validateRegistryAction(action string) error {
	if action == "read" || action == "write" {
		return nil
	}
	return fmt.Errorf("invalid registry action: %s", action)
}
