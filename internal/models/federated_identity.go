package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FederatedIdentity binds an external OIDC identity to a Nebi user.
type FederatedIdentity struct {
	ID            uuid.UUID `gorm:"type:text;primary_key" json:"id"`
	UserID        uuid.UUID `gorm:"type:text;not null;index" json:"user_id"`
	User          User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Issuer        string    `gorm:"not null;uniqueIndex:idx_federated_identity_issuer_subject" json:"issuer"`
	Subject       string    `gorm:"not null;uniqueIndex:idx_federated_identity_issuer_subject" json:"subject"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Name          string    `json:"name"`
	AvatarURL     string    `json:"avatar_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BeforeCreate hook to generate UUID.
func (f *FederatedIdentity) BeforeCreate(tx *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}
