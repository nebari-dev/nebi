package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User is a person known to nebi. Team-mode users are provisioned from the
// identity provider on first sight (see FederatedIdentity).
type User struct {
	ID        uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Username  string         `gorm:"uniqueIndex;not null" json:"username"`
	Email     string         `gorm:"uniqueIndex;not null" json:"email"`
	AvatarURL string         `json:"avatar_url"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
