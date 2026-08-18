package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	FederatedIdentityReviewStatusPending  = "pending"
	FederatedIdentityReviewStatusRejected = "rejected"

	FederatedIdentityReviewCollisionUsername      = "username"
	FederatedIdentityReviewCollisionEmail         = "email"
	FederatedIdentityReviewCollisionUsernameEmail = "username,email"
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

// FederatedIdentityReview records a blocked automatic account-linking
// decision that requires an administrator to approve deliberately.
type FederatedIdentityReview struct {
	ID             uuid.UUID `gorm:"type:text;primary_key" json:"id"`
	UserID         uuid.UUID `gorm:"type:text;not null;index" json:"user_id"`
	User           User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user"`
	Issuer         string    `gorm:"not null;uniqueIndex:idx_federated_identity_review_issuer_subject" json:"issuer"`
	Subject        string    `gorm:"not null;uniqueIndex:idx_federated_identity_review_issuer_subject" json:"subject"`
	CollisionField string    `gorm:"not null" json:"collision_field"`
	// User matched by the incoming username claim, when present.
	CollisionUsernameUserID *uuid.UUID `gorm:"type:text;index" json:"collision_username_user_id,omitempty"`
	CollisionUsernameUser   *User      `gorm:"foreignKey:CollisionUsernameUserID" json:"collision_username_user,omitempty"`
	// User matched by the incoming verified email claim, when present.
	CollisionEmailUserID *uuid.UUID `gorm:"type:text;index" json:"collision_email_user_id,omitempty"`
	CollisionEmailUser   *User      `gorm:"foreignKey:CollisionEmailUserID" json:"collision_email_user,omitempty"`
	Username             string     `json:"username"`
	Email                string     `json:"email"`
	EmailVerified        bool       `json:"email_verified"`
	Name                 string     `json:"name"`
	AvatarURL            string     `json:"avatar_url"`
	Status               string     `gorm:"not null;default:pending;index" json:"status"`
	ReviewedBy           *uuid.UUID `gorm:"type:text;index" json:"reviewed_by,omitempty"`
	ReviewedAt           *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// BeforeCreate hook to generate UUID.
func (f *FederatedIdentityReview) BeforeCreate(tx *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}

// IsPending reports whether the review is still awaiting an admin decision.
// Empty status is treated as pending for rows created before the status column
// had an explicit default.
func (f FederatedIdentityReview) IsPending() bool {
	return f.Status == "" || f.Status == FederatedIdentityReviewStatusPending
}

// HasAmbiguousCollision reports whether username and verified-email claims
// matched different existing users.
func (f FederatedIdentityReview) HasAmbiguousCollision() bool {
	return f.CollisionUsernameUserID != nil &&
		f.CollisionEmailUserID != nil &&
		*f.CollisionUsernameUserID != *f.CollisionEmailUserID
}
