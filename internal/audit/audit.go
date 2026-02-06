package audit

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
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
	ActionCreateUser        = "create_user"
	ActionUpdateUser        = "update_user"
	ActionDeleteUser        = "delete_user"
	ActionMakeAdmin         = "make_admin"
	ActionRevokeAdmin       = "revoke_admin"
	ActionGrantPermission   = "grant_permission"
	ActionRevokePermission  = "revoke_permission"
	ActionCreateWorkspace   = "create_workspace"
	ActionDeleteWorkspace   = "delete_workspace"
	ActionInstallPackage    = "install_package"
	ActionRemovePackage     = "remove_package"
	ActionPublishWorkspace  = "publish_workspace"
	ActionPush              = "push"
	ActionReassignTag       = "reassign_tag"
	ActionLogin             = "login"
	ActionLoginFailed       = "login_failed"
)

// Resource types
const (
	ResourceUser       = "user"
	ResourceWorkspace  = "workspace"
	ResourcePermission = "permission"
)

// Log is a convenience function for logging with resource ID
func Log(db *gorm.DB, userID uuid.UUID, action, resource string, resourceID uuid.UUID, details map[string]interface{}) error {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["resource_id"] = resourceID.String()
	return LogAction(db, userID, action, resource, details)
}
