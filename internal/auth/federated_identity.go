package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errFederatedIdentityRequiresReview = errors.New("federated identity requires admin review")
var errFederatedIdentityRejected = errors.New("federated identity review was rejected")
var errRetryFederatedIdentityResolution = errors.New("retry federated identity resolution")

const (
	FederatedIdentityReviewPendingCode  = "identity_review_pending"
	FederatedIdentityReviewRejectedCode = "identity_review_rejected"
)

// FederatedIdentityReviewErrorCode returns a stable client-facing error code
// for review-gated federated login failures.
func FederatedIdentityReviewErrorCode(err error) (string, bool) {
	// Keep API responses on stable codes while sentinel errors stay internal.
	if errors.Is(err, errFederatedIdentityRequiresReview) {
		return FederatedIdentityReviewPendingCode, true
	}
	if errors.Is(err, errFederatedIdentityRejected) {
		return FederatedIdentityReviewRejectedCode, true
	}
	return "", false
}

type federatedUserClaims struct {
	Issuer            string
	Subject           string
	PreferredUsername string
	Email             string
	EmailVerified     bool
	Name              string
	AvatarURL         string
}

func findOrCreateFederatedUser(db *gorm.DB, claims federatedUserClaims) (*models.User, error) {
	claims = normalizeFederatedClaims(claims)
	if claims.Issuer == "" {
		return nil, errors.New("federated token has no issuer")
	}
	if claims.Subject == "" {
		return nil, errors.New("federated token has no subject")
	}

	user, created, err := findOrCreateFederatedUserOnce(db, claims)
	if errors.Is(err, errRetryFederatedIdentityResolution) {
		user, created, err = findOrCreateFederatedUserOnce(db, claims)
	}
	if err != nil {
		return nil, err
	}
	if created {
		slog.Info("Created new user from federated identity", "user_id", user.ID, "username", user.Username)
	}
	return user, nil
}

func findOrCreateFederatedUserOnce(db *gorm.DB, claims federatedUserClaims) (*models.User, bool, error) {
	suffix := federatedIdentitySuffix(claims.Issuer, claims.Subject)
	var user models.User
	var created bool
	var reviewRequired error
	if err := db.Transaction(func(tx *gorm.DB) error {
		found, err := loadFederatedIdentityUser(tx, claims, &user)
		if err != nil {
			return err
		}
		if found {
			return nil
		}

		if err := enforceRejectedFederatedIdentityReview(tx, claims); err != nil {
			return err
		}

		collision, field, err := federatedReviewCollision(tx, claims)
		if err != nil {
			return err
		}
		if collision != nil {
			if err := recordFederatedIdentityReview(tx, collision.ID, field, claims); err != nil {
				return err
			}
			reviewRequired = fmt.Errorf("%w: existing user %s has a matching %s claim", errFederatedIdentityRequiresReview, collision.ID, field)
			return nil
		}
		if err := deleteStalePendingFederatedIdentityReview(tx, claims); err != nil {
			return err
		}

		username, err := federatedUserUsername(tx, claims, suffix)
		if err != nil {
			return err
		}
		email, err := federatedUserEmail(tx, claims, suffix)
		if err != nil {
			return err
		}

		user = models.User{
			Username:     username,
			Email:        email,
			AvatarURL:    claims.AvatarURL,
			PasswordHash: "",
		}
		if err := tx.Create(&user).Error; err != nil {
			if isUniqueConstraintError(err) {
				return errRetryFederatedIdentityResolution
			}
			return fmt.Errorf("failed to create federated user: %w", err)
		}

		identity := models.FederatedIdentity{
			UserID:        user.ID,
			Issuer:        claims.Issuer,
			Subject:       claims.Subject,
			Username:      claims.PreferredUsername,
			Email:         claims.Email,
			EmailVerified: claims.EmailVerified,
			Name:          claims.Name,
			AvatarURL:     claims.AvatarURL,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "issuer"}, {Name: "subject"}},
			DoNothing: true,
		}).Create(&identity)
		if result.Error != nil {
			return fmt.Errorf("failed to create federated identity: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errRetryFederatedIdentityResolution
		}
		created = true
		return nil
	}); err != nil {
		return nil, false, err
	}
	if reviewRequired != nil {
		return nil, false, reviewRequired
	}

	return &user, created, nil
}

func loadFederatedIdentityUser(db *gorm.DB, claims federatedUserClaims, user *models.User) (bool, error) {
	var identity models.FederatedIdentity
	err := db.Preload("User").
		Where("issuer = ? AND subject = ?", claims.Issuer, claims.Subject).
		First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("database error: %w", err)
	}

	loadedUser := identity.User
	if loadedUser.ID != identity.UserID {
		if err := db.Unscoped().Delete(&identity).Error; err != nil {
			return false, fmt.Errorf("delete orphaned federated identity: %w", err)
		}
		return false, nil
	}
	if loadedUser.AvatarURL != claims.AvatarURL {
		if err := db.Model(&loadedUser).Update("avatar_url", claims.AvatarURL).Error; err != nil {
			return false, fmt.Errorf("failed to update federated user profile: %w", err)
		}
		loadedUser.AvatarURL = claims.AvatarURL
	}
	if err := updateFederatedIdentityProfile(db, &identity, claims); err != nil {
		return false, err
	}
	*user = loadedUser
	return true, nil
}

func normalizeFederatedClaims(claims federatedUserClaims) federatedUserClaims {
	claims.Issuer = strings.TrimSpace(claims.Issuer)
	claims.Subject = strings.TrimSpace(claims.Subject)
	claims.PreferredUsername = strings.ToLower(strings.TrimSpace(claims.PreferredUsername))
	claims.Email = strings.ToLower(strings.TrimSpace(claims.Email))
	claims.Name = strings.TrimSpace(claims.Name)
	claims.AvatarURL = strings.TrimSpace(claims.AvatarURL)
	return claims
}

func federatedReviewCollision(db *gorm.DB, claims federatedUserClaims) (*models.User, string, error) {
	if claims.PreferredUsername != "" {
		user, err := userByField(db, "username", claims.PreferredUsername)
		if err != nil {
			return nil, "", err
		}
		if user != nil {
			return user, "username", nil
		}
	}

	if claims.Email != "" {
		user, err := userByField(db, "email", claims.Email)
		if err != nil {
			return nil, "", err
		}
		if user != nil {
			return user, "email", nil
		}
	}

	return nil, "", nil
}

func enforceRejectedFederatedIdentityReview(db *gorm.DB, claims federatedUserClaims) error {
	var review models.FederatedIdentityReview
	err := db.
		Where("issuer = ? AND subject = ? AND status = ?", claims.Issuer, claims.Subject, models.FederatedIdentityReviewStatusRejected).
		First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check federated identity review: %w", err)
	}
	return fmt.Errorf("%w: issuer %s subject %s", errFederatedIdentityRejected, claims.Issuer, claims.Subject)
}

func recordFederatedIdentityReview(db *gorm.DB, userID uuid.UUID, field string, claims federatedUserClaims) error {
	var review models.FederatedIdentityReview
	err := db.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Subject).First(&review).Error
	if err == nil {
		if review.Status == models.FederatedIdentityReviewStatusRejected {
			return fmt.Errorf("%w: issuer %s subject %s", errFederatedIdentityRejected, claims.Issuer, claims.Subject)
		}
		// Keep pending reviews bound to the user and claims an admin reviewed.
		// Profile details refresh after approval through updateFederatedIdentityProfile.
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to record federated identity review: %w", err)
	}
	review = models.FederatedIdentityReview{
		UserID:         userID,
		Issuer:         claims.Issuer,
		Subject:        claims.Subject,
		CollisionField: field,
		Username:       claims.PreferredUsername,
		Email:          claims.Email,
		EmailVerified:  claims.EmailVerified,
		Name:           claims.Name,
		AvatarURL:      claims.AvatarURL,
		Status:         models.FederatedIdentityReviewStatusPending,
	}
	if err := db.Create(&review).Error; err != nil {
		return fmt.Errorf("failed to record federated identity review: %w", err)
	}
	return nil
}

func deleteStalePendingFederatedIdentityReview(db *gorm.DB, claims federatedUserClaims) error {
	if err := db.Unscoped().
		Where("issuer = ? AND subject = ?", claims.Issuer, claims.Subject).
		Where("status = ? OR status = ''", models.FederatedIdentityReviewStatusPending).
		Delete(&models.FederatedIdentityReview{}).Error; err != nil {
		return fmt.Errorf("delete stale federated identity review: %w", err)
	}
	return nil
}

func userByField(db *gorm.DB, field, value string) (*models.User, error) {
	if field != "username" && field != "email" {
		return nil, fmt.Errorf("unsupported user collision field: %s", field)
	}

	var user models.User
	err := db.Where("LOWER("+field+") = LOWER(?)", value).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check existing user %s collision: %w", field, err)
	}

	return &user, nil
}

func updateFederatedIdentityProfile(db *gorm.DB, identity *models.FederatedIdentity, claims federatedUserClaims) error {
	if identity.Username == claims.PreferredUsername &&
		identity.Email == claims.Email &&
		identity.EmailVerified == claims.EmailVerified &&
		identity.Name == claims.Name &&
		identity.AvatarURL == claims.AvatarURL {
		return nil
	}

	updates := map[string]any{
		"username":       claims.PreferredUsername,
		"email":          claims.Email,
		"email_verified": claims.EmailVerified,
		"name":           claims.Name,
		"avatar_url":     claims.AvatarURL,
	}
	if err := db.Model(&models.FederatedIdentity{}).
		Where("id = ?", identity.ID).
		Select("username", "email", "email_verified", "name", "avatar_url").
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update federated identity profile: %w", err)
	}

	identity.Username = claims.PreferredUsername
	identity.Email = claims.Email
	identity.EmailVerified = claims.EmailVerified
	identity.Name = claims.Name
	identity.AvatarURL = claims.AvatarURL
	return nil
}

func federatedUserUsername(db *gorm.DB, claims federatedUserClaims, suffix string) (string, error) {
	if claims.PreferredUsername != "" {
		base := claims.PreferredUsername
		return uniqueUserValue(usernameExists, db, base, func(n int) string {
			return fmt.Sprintf("%s-%s-%d", base, suffix, n)
		})
	}

	base := claims.Subject
	if claims.EmailVerified && claims.Email != "" {
		base = claims.Email
	}
	return uniqueUserValue(usernameExists, db, base, func(n int) string {
		return fmt.Sprintf("%s-%s-%d", base, suffix, n)
	})
}

func federatedUserEmail(db *gorm.DB, claims federatedUserClaims, suffix string) (string, error) {
	if claims.Email != "" {
		base := claims.Email
		return uniqueUserValue(emailExists, db, base, func(n int) string {
			return federatedEmailWithSuffix(base, suffix, n)
		})
	}

	base := claims.PreferredUsername
	if base == "" {
		base = claims.Subject
	}
	return uniqueUserValue(emailExists, db, base+"@nebi.local", func(n int) string {
		return fmt.Sprintf("%s-%d@nebi.local", base, n)
	})
}

func uniqueUserValue(existsFn func(*gorm.DB, string) (bool, error), db *gorm.DB, first string, next func(int) string) (string, error) {
	for i := 0; ; i++ {
		value := first
		if i > 0 {
			value = next(i)
		}
		exists, err := existsFn(db, value)
		if err != nil {
			return "", err
		}
		if !exists {
			return value, nil
		}
	}
}

func usernameExists(db *gorm.DB, value string) (bool, error) {
	var count int64
	if err := db.Unscoped().Model(&models.User{}).Where("LOWER(username) = LOWER(?)", value).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check user username collision: %w", err)
	}
	return count > 0, nil
}

func emailExists(db *gorm.DB, value string) (bool, error) {
	var count int64
	if err := db.Unscoped().Model(&models.User{}).Where("LOWER(email) = LOWER(?)", value).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check user email collision: %w", err)
	}
	return count > 0, nil
}

func federatedEmailWithSuffix(email, suffix string, n int) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return fmt.Sprintf("%s-%s-%d@nebi.local", email, suffix, n)
	}
	return fmt.Sprintf("%s+%s-%d@%s", local, suffix, n, domain)
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key value violates unique constraint") ||
		strings.Contains(msg, "constraint failed")
}

func federatedIdentitySuffix(issuer, subject string) string {
	sum := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return hex.EncodeToString(sum[:])[:12]
}
