package store

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"gorm.io/gorm"
)

// LocalUser mirrors models.User for CLI/local use. It shares the "users"
// table with the server-side model so the GUI and CLI can reference the
// same user (workspace_versions.created_by and workspaces.owner_id both
// foreign-key into this table).
type LocalUser struct {
	ID           uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Username     string         `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string         `gorm:"not null" json:"-"`
	Email        string         `gorm:"uniqueIndex;not null" json:"email"`
	AvatarURL    string         `json:"avatar_url"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName ensures GORM uses the "users" table.
func (LocalUser) TableName() string {
	return "users"
}

// BeforeCreate generates the UUID.
func (u *LocalUser) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

// ensureLocalUser finds or creates the well-known local-user row and
// returns its ID. Used as the created_by / owner_id for records inserted
// from the CLI in local mode. The username matches auth.LocalUsername()
// so the CLI and server share the same user row.
func ensureLocalUser(db *gorm.DB) (uuid.UUID, error) {
	username := auth.LocalUsername()
	var user LocalUser
	err := db.Where("username = ?", username).First(&user).Error
	if err == nil {
		return user.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, err
	}

	user = LocalUser{
		Username:     username,
		Email:        username + "@nebi.local",
		PasswordHash: "-",
	}
	if err := db.Create(&user).Error; err != nil {
		return uuid.Nil, err
	}
	return user.ID, nil
}
