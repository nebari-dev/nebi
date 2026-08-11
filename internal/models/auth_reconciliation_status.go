package models

import (
	"time"

	"github.com/google/uuid"
)

// AuthReconciliationStatus records the last successful and failed
// authorization-state reconciliation for a user and reconciliation kind.
type AuthReconciliationStatus struct {
	UserID              uuid.UUID  `gorm:"type:text;primary_key" json:"user_id"`
	Kind                string     `gorm:"type:text;primary_key" json:"kind"`
	LastSuccessAt       *time.Time `gorm:"index" json:"last_success_at,omitempty"`
	LastFailureAt       *time.Time `gorm:"index" json:"last_failure_at,omitempty"`
	LastFailureSource   string     `gorm:"type:text;index" json:"last_failure_source,omitempty"`
	ConsecutiveFailures int        `gorm:"not null;default:0" json:"consecutive_failures"`
	LastError           string     `gorm:"type:text" json:"last_error,omitempty"`
	DesiredGroupsJSON   string     `gorm:"type:text" json:"desired_groups_json,omitempty"`
	DesiredAdmin        *bool      `json:"desired_admin,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	User                User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
