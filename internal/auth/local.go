package auth

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

const localUsername = "local-user"

// LocalAuthenticator provides a no-op authenticator for local/desktop mode.
// It ensures a well-known "local-user" exists in the database and injects
// that user into every request context without checking credentials.
type LocalAuthenticator struct {
	user *models.User
}

// NewLocalAuthenticator finds or creates the well-known local-user and
// returns an authenticator that always uses that user.
func NewLocalAuthenticator(db *gorm.DB) (*LocalAuthenticator, error) {
	var user models.User
	err := db.Where("username = ?", localUsername).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = models.User{
			Username:     localUsername,
			Email:        localUsername + "@nebi.local",
			PasswordHash: "-", // no password; local mode never checks credentials
		}
		if err := db.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to create local-user: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to look up local-user: %w", err)
	}

	return &LocalAuthenticator{user: &user}, nil
}

// Login returns the local-user with a dummy token (no password check).
func (a *LocalAuthenticator) Login(_, _ string) (*LoginResponse, error) {
	return &LoginResponse{
		Token: "local-mode-token",
		User:  a.user,
	}, nil
}

// Middleware injects the local-user into the context without checking credentials.
func (a *LocalAuthenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(UserContextKey, a.user)
		c.Next()
	}
}

// GetUserFromContext extracts the authenticated user from the Gin context.
func (a *LocalAuthenticator) GetUserFromContext(c *gin.Context) (*models.User, error) {
	value, exists := c.Get(UserContextKey)
	if !exists {
		return nil, ErrUnauthorized
	}
	user, ok := value.(*models.User)
	if !ok {
		return nil, errors.New("invalid user in context")
	}
	return user, nil
}

// User returns the local-user for use outside the HTTP request path
// (e.g. granting RBAC roles at startup).
func (a *LocalAuthenticator) User() *models.User {
	return a.user
}

// LocalUsername is the well-known username used in local mode.
func LocalUsername() string {
	return localUsername
}
