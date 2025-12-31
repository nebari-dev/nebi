package audit

import (
	"encoding/json"
	"time"

	"github.com/aktech/darb/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LogAction records an audit log entry
func LogAction(db *gorm.DB, userID uuid.UUID, action, resource string, details interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	log := models.AuditLog{
		UserID:      userID,
		Action:      action,
		Resource:    resource,
		DetailsJSON: string(detailsJSON),
		Timestamp:   time.Now(),
	}

	return db.Create(&log).Error
}

// Audit actions constants
const (
	ActionCreateUser         = "create_user"
	ActionUpdateUser         = "update_user"
	ActionDeleteUser         = "delete_user"
	ActionMakeAdmin          = "make_admin"
	ActionRevokeAdmin        = "revoke_admin"
	ActionGrantPermission    = "grant_permission"
	ActionRevokePermission   = "revoke_permission"
	ActionCreateEnvironment  = "create_environment"
	ActionDeleteEnvironment  = "delete_environment"
	ActionInstallPackage     = "install_package"
	ActionRemovePackage      = "remove_package"
	ActionPublishEnvironment = "publish_environment"
	ActionLogin              = "login"
	ActionLoginFailed        = "login_failed"
)

// Resource types
const (
	ResourceUser        = "user"
	ResourceEnvironment = "environment"
	ResourcePermission  = "permission"
)

// Log is a convenience function for logging with resource ID
func Log(db *gorm.DB, userID uuid.UUID, action, resource string, resourceID uuid.UUID, details map[string]interface{}) error {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["resource_id"] = resourceID.String()
	return LogAction(db, userID, action, resource, details)
}
