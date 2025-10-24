package models

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog represents a record of user actions for compliance
type AuditLog struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	UserID      uuid.UUID `gorm:"type:text;index" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Action      string    `gorm:"not null" json:"action"`        // e.g., "create_environment", "grant_permission"
	Resource    string    `gorm:"not null" json:"resource"`      // e.g., "environment:123", "user:456"
	DetailsJSON string    `gorm:"type:text" json:"details_json"` // Additional context in JSON
	Timestamp   time.Time `gorm:"not null;index" json:"timestamp"`
}
