package auth

import (
	"errors"
	"strings"
	"sync"

	"github.com/aktech/darb/internal/models"
	"github.com/gin-gonic/gin"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
)

// Local token support for CLI auto-spawned servers
var (
	localToken   string
	localTokenMu sync.RWMutex
)

// SetLocalToken sets the local token for CLI authentication
// This token bypasses normal JWT validation and is used for local server mode
func SetLocalToken(token string) {
	localTokenMu.Lock()
	defer localTokenMu.Unlock()
	localToken = token
}

// GetLocalToken returns the current local token
func GetLocalToken() string {
	localTokenMu.RLock()
	defer localTokenMu.RUnlock()
	return localToken
}

// IsLocalToken checks if the given token is a valid local token
func IsLocalToken(token string) bool {
	localTokenMu.RLock()
	defer localTokenMu.RUnlock()
	return localToken != "" && token == localToken && strings.HasPrefix(token, "nebi_local_")
}

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

// Authenticator is an interface for authentication providers
type Authenticator interface {
	// Login authenticates a user and returns a JWT token
	Login(username, password string) (*LoginResponse, error)

	// Middleware returns a Gin middleware for authentication
	Middleware() gin.HandlerFunc

	// GetUserFromContext extracts the authenticated user from the Gin context
	GetUserFromContext(c *gin.Context) (*models.User, error)
}
