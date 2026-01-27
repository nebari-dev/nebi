package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// LocalTokenAuthenticator wraps another authenticator and also accepts a local token.
// When the local token is provided, it authenticates as a local admin user without
// requiring JWT validation. This is used for local server mode where the CLI connects
// using the token from server.state.
type LocalTokenAuthenticator struct {
	inner      Authenticator
	localToken string
	db         *gorm.DB
}

// NewLocalTokenAuthenticator creates a new authenticator that accepts a local token
// in addition to the underlying authenticator's tokens.
func NewLocalTokenAuthenticator(inner Authenticator, localToken string, db *gorm.DB) *LocalTokenAuthenticator {
	return &LocalTokenAuthenticator{
		inner:      inner,
		localToken: localToken,
		db:         db,
	}
}

// Login delegates to the inner authenticator.
func (a *LocalTokenAuthenticator) Login(username, password string) (*LoginResponse, error) {
	return a.inner.Login(username, password)
}

// Middleware returns a Gin middleware that accepts the local token or delegates to the inner auth.
func (a *LocalTokenAuthenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header.
		authHeader := c.GetHeader("Authorization")
		var tokenString string
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		} else {
			// Fallback to query parameter (for EventSource/SSE compatibility).
			tokenString = c.Query("token")
		}

		// Check if this is the local token.
		if tokenString != "" && tokenString == a.localToken {
			// Authenticate as local admin user.
			user := a.getOrCreateLocalUser()
			if user != nil {
				c.Set(UserContextKey, user)
				c.Next()
				return
			}
			slog.Error("Failed to get/create local user")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			c.Abort()
			return
		}

		// Delegate to inner authenticator.
		a.inner.Middleware()(c)
	}
}

// GetUserFromContext delegates to the inner authenticator.
func (a *LocalTokenAuthenticator) GetUserFromContext(c *gin.Context) (*models.User, error) {
	return a.inner.GetUserFromContext(c)
}

// getOrCreateLocalUser finds or creates the local admin user.
// The local user is granted admin privileges so it can perform all operations.
func (a *LocalTokenAuthenticator) getOrCreateLocalUser() *models.User {
	var user models.User
	result := a.db.Where("username = ?", "local").First(&user)
	if result.Error == nil {
		// Ensure admin privileges are set (idempotent).
		a.ensureAdmin(user.ID)
		return &user
	}

	// Create the local user.
	user = models.User{
		ID:       uuid.New(),
		Username: "local",
		Email:    "local@localhost",
	}
	if err := a.db.Create(&user).Error; err != nil {
		// Maybe another goroutine created it; try to fetch again.
		if a.db.Where("username = ?", "local").First(&user).Error == nil {
			a.ensureAdmin(user.ID)
			return &user
		}
		slog.Error("Failed to create local user", "error", err)
		return nil
	}

	// Grant admin privileges.
	a.ensureAdmin(user.ID)
	return &user
}

// ensureAdmin grants admin privileges to the user if not already granted.
func (a *LocalTokenAuthenticator) ensureAdmin(userID uuid.UUID) {
	if err := rbac.MakeAdmin(userID); err != nil {
		slog.Error("Failed to grant admin privileges to local user", "error", err)
	}
}
