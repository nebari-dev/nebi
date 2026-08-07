package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BuildEnvVar stores an encrypted per-user environment variable that is
// injected into package-manager build commands. Values are never exposed.
type BuildEnvVar struct {
	ID        uuid.UUID `gorm:"type:text;primary_key" json:"id"`
	UserID    uuid.UUID `gorm:"type:text;not null;index;uniqueIndex:idx_build_env_var_user_key" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Key       string    `gorm:"not null;uniqueIndex:idx_build_env_var_user_key" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName ensures GORM uses the "build_env_vars" table.
func (BuildEnvVar) TableName() string {
	return "build_env_vars"
}

// BeforeCreate hook to generate UUID.
func (v *BuildEnvVar) BeforeCreate(tx *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return nil
}
