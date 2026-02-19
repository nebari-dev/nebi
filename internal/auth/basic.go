package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
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
	db               *gorm.DB
	jwtSecret        []byte
	proxyAdminGroups []string
}

// NewBasicAuthenticator creates a new basic authenticator
func NewBasicAuthenticator(db *gorm.DB, jwtSecret string) *BasicAuthenticator {
	return &BasicAuthenticator{
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}
}

// SetProxyAdminGroups configures which IdToken groups grant Nebi admin.
func (a *BasicAuthenticator) SetProxyAdminGroups(groups string) {
	a.proxyAdminGroups = parseAdminGroups(groups)
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
			Issuer:    "nebi",
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

// Middleware returns a Gin middleware for authentication.
// It checks (in order): Bearer token header, ?token= query param, IdToken cookie.
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
		}

		// If we have a Nebi JWT, validate it
		if tokenString != "" {
			user, err := a.validateAndLoadUser(tokenString)
			if err != nil {
				slog.Warn("Invalid token", "error", err)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
				c.Abort()
				return
			}
			c.Set(UserContextKey, user)
			c.Next()
			return
		}

		// Fallback: try IdToken cookie from authenticating proxy (e.g. Envoy Gateway)
		proxyClaims, err := parseIdTokenCookie(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization"})
			c.Abort()
			return
		}

		user, err := findOrCreateProxyUser(a.db, proxyClaims)
		if err != nil {
			slog.Error("Failed to find/create proxy user", "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "proxy authentication failed"})
			c.Abort()
			return
		}

		// Sync admin role from proxy groups on every request
		syncRolesFromGroups(user.ID, proxyClaims.Groups, a.proxyAdminGroups)

		c.Set(UserContextKey, user)
		c.Next()
	}
}

// validateAndLoadUser validates a Nebi JWT and loads the user from the database.
func (a *BasicAuthenticator) validateAndLoadUser(tokenString string) (*models.User, error) {
	claims, err := a.validateToken(tokenString)
	if err != nil {
		return nil, err
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in token: %w", err)
	}

	var user models.User
	if result := a.db.First(&user, userID); result.Error != nil {
		return nil, fmt.Errorf("user not found: %w", result.Error)
	}

	return &user, nil
}

// SessionFromProxy checks for an IdToken cookie, finds/creates the user,
// syncs roles, and returns a Nebi JWT + user. Used by /auth/session.
func (a *BasicAuthenticator) SessionFromProxy(r *http.Request, adminGroups string) (*LoginResponse, error) {
	proxyClaims, err := parseIdTokenCookie(r)
	if err != nil {
		return nil, err
	}

	user, err := findOrCreateProxyUser(a.db, proxyClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to find/create proxy user: %w", err)
	}

	syncRolesFromGroups(user.ID, proxyClaims.Groups, parseAdminGroups(adminGroups))

	token, err := a.generateToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResponse{
		Token: token,
		User:  user,
	}, nil
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
