package auth

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/models"
)

// NOTE(intermediate): UserContextKey is still defined in basic.go; do not add it
// here until basic.go is removed.

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrAuthorizationStale = errors.New("authorization reconciliation is stale")
)

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

// Authenticator resolves the user behind a request.
type Authenticator interface {
	// Middleware authenticates the request and stores the user under
	// UserContextKey, or aborts it.
	Middleware() gin.HandlerFunc
}

// UserFromContext returns the user stored by an Authenticator's middleware.
func UserFromContext(c *gin.Context) (*models.User, error) {
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
