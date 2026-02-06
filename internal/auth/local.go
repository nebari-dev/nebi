package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// LocalAuthenticator provides a no-op authenticator for local/desktop mode.
// It automatically injects the first admin user into every request context
// without checking credentials.
type LocalAuthenticator struct {
	db *gorm.DB
}

// NewLocalAuthenticator creates a new local authenticator.
func NewLocalAuthenticator(db *gorm.DB) *LocalAuthenticator {
	return &LocalAuthenticator{db: db}
}

// Login returns the first user with a dummy token (no password check).
func (a *LocalAuthenticator) Login(username, password string) (*LoginResponse, error) {
	var user models.User
	if err := a.db.First(&user).Error; err != nil {
		return nil, errors.New("no users in database")
	}
	return &LoginResponse{
		Token: "local-mode-token",
		User:  &user,
	}, nil
}

// Middleware injects the first admin user into the context without checking credentials.
func (a *LocalAuthenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var user models.User
		if err := a.db.First(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no users configured"})
			c.Abort()
			return
		}
		c.Set(UserContextKey, &user)
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
