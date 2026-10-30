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
	ActionGrantPermission      = "grant_permission"
	ActionRevokePermission     = "revoke_permission"
	ActionCreateGroup          = "create_group"
	ActionAddGroupMember       = "add_group_member"
	ActionRemoveGroupMember    = "remove_group_member"
	ActionGrantGroupPerm       = "grant_group_permission"
	ActionRevokeGroupPerm      = "revoke_group_permission"
	ActionCreateProject        = "create_project"
	ActionDeleteProject        = "delete_project"
	ActionInstallPackage       = "install_package"
	ActionRemovePackage        = "remove_package"
	ActionSolveProject         = "solve_project"
	ActionRollbackProject      = "rollback_project"
	ActionInstallEnv           = "install_environment"
	ActionUninstallEnv         = "uninstall_environment"
	ActionPublishProject       = "publish_project"
	ActionImportProject        = "import_project"
	ActionRegistryAccessDenied = "registry_access_denied"
	ActionPush                 = "push"
	ActionReassignTag          = "reassign_tag"
)

// Resource types
const (
	ResourceProject = "project"
)

// Log is a convenience function for logging with resource ID
func Log(db *gorm.DB, userID uuid.UUID, action, resource string, resourceID uuid.UUID, details map[string]interface{}) error {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["resource_id"] = resourceID.String()
	return LogAction(db, userID, action, resource, details)
}
