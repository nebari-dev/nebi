package auth

import (
	"errors"

	"github.com/nebari-dev/nebi/internal/models"
	"github.com/gin-gonic/gin"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
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

// Authenticator is an interface for authentication providers
type Authenticator interface {
	// Login authenticates a user and returns a JWT token
	Login(username, password string) (*LoginResponse, error)

	// Middleware returns a Gin middleware for authentication
	Middleware() gin.HandlerFunc

	// GetUserFromContext extracts the authenticated user from the Gin context
	GetUserFromContext(c *gin.Context) (*models.User, error)
}
