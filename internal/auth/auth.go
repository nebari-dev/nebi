package auth

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/models"
)

// UserContextKey is the key used to store the authenticated user in the Gin context.
const UserContextKey = "user"

var ErrUnauthorized = errors.New("unauthorized")

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
