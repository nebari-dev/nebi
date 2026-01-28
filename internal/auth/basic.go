package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aktech/darb/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	// UserContextKey is the key used to store user in Gin context
	UserContextKey = "user"
	// TokenDuration is the validity period for JWT tokens
	TokenDuration = 24 * time.Hour
)

// BasicAuthenticator implements basic username/password authentication
type BasicAuthenticator struct {
	db        *gorm.DB
	jwtSecret []byte
}

// NewBasicAuthenticator creates a new basic authenticator
func NewBasicAuthenticator(db *gorm.DB, jwtSecret string) *BasicAuthenticator {
	return &BasicAuthenticator{
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword checks if a password matches the hash
func VerifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Claims represents JWT claims
type Claims struct {
	UserID   string `json:"user_id"` // UUID stored as string
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Login authenticates a user and returns a JWT token
func (a *BasicAuthenticator) Login(username, password string) (*LoginResponse, error) {
	var user models.User
	result := a.db.Where("username = ?", username).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			slog.Warn("Login attempt with non-existent username", "username", username)
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("database error: %w", result.Error)
	}

	// Verify password
	if !VerifyPassword(user.PasswordHash, password) {
		slog.Warn("Login attempt with incorrect password", "username", username)
		return nil, ErrInvalidCredentials
	}

	// Generate JWT token
	token, err := a.generateToken(&user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	slog.Info("User logged in successfully", "user_id", user.ID, "username", user.Username)
	return &LoginResponse{
		Token: token,
		User:  &user,
	}, nil
}

// generateToken creates a JWT token for a user
func (a *BasicAuthenticator) generateToken(user *models.User) (string, error) {
	claims := Claims{
		UserID:   user.ID.String(),
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "darb",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(a.jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// validateToken validates a JWT token and returns claims
func (a *BasicAuthenticator) validateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrUnauthorized
}

// Middleware returns a Gin middleware for authentication
func (a *BasicAuthenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string

		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			// Check for Bearer token
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
				c.Abort()
				return
			}
			tokenString = parts[1]
		} else {
			// Fallback to query parameter (for EventSource/SSE compatibility)
			tokenString = c.Query("token")
			if tokenString == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization"})
				c.Abort()
				return
			}
		}

		// Validate token
		claims, err := a.validateToken(tokenString)
		if err != nil {
			slog.Warn("Invalid token", "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		// Parse user ID from claims
		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			slog.Error("Invalid user ID in token", "user_id", claims.UserID, "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID"})
			c.Abort()
			return
		}

		// Load user from database
		var user models.User
		result := a.db.First(&user, userID)
		if result.Error != nil {
			slog.Error("Failed to load user from token", "user_id", userID, "error", result.Error)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token - please run 'nebi login' again"})
			c.Abort()
			return
		}

		// Store user in context
		c.Set(UserContextKey, &user)
		c.Next()
	}
}

// GetUserFromContext extracts the authenticated user from the Gin context
func (a *BasicAuthenticator) GetUserFromContext(c *gin.Context) (*models.User, error) {
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
