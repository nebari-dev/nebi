package auth

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// LocalAuthenticator bypasses authentication for local/desktop mode.
// It injects the first admin user into the request context without
// requiring any credentials.
type LocalAuthenticator struct {
	db *gorm.DB
}

// NewLocalAuthenticator creates a new local mode authenticator
func NewLocalAuthenticator(db *gorm.DB) *LocalAuthenticator {
	return &LocalAuthenticator{db: db}
}

// Login returns the admin user without checking credentials
func (a *LocalAuthenticator) Login(username, password string) (*LoginResponse, error) {
	user, err := a.getAdminUser()
	if err != nil {
		return nil, err
	}
	return &LoginResponse{
		Token: "local-mode",
		User:  user,
	}, nil
}

// Middleware returns a Gin middleware that injects the admin user into context
// without requiring any authentication token
func (a *LocalAuthenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := a.getAdminUser()
		if err != nil {
			slog.Error("Local mode: failed to find admin user", "error", err)
			c.Next()
			return
		}
		c.Set(UserContextKey, user)
		c.Next()
	}
}

// GetUserFromContext extracts the user from the Gin context
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

// getAdminUser finds the first user in the database (the auto-created admin)
func (a *LocalAuthenticator) getAdminUser() (*models.User, error) {
	var user models.User
	if err := a.db.Order("created_at ASC").First(&user).Error; err != nil {
		return nil, errors.New("no users found; ensure admin user is created")
	}
	return &user, nil
}
