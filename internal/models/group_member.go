package models

import (
	"time"

	"github.com/google/uuid"
)

// GroupMember is the join table linking users to groups. Composite primary key
// (group_id, user_id) — a user appears at most once per group.
type GroupMember struct {
	GroupID   uuid.UUID `gorm:"type:text;primary_key" json:"group_id"`
	UserID    uuid.UUID `gorm:"type:text;primary_key" json:"user_id"`
	Group     Group     `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
