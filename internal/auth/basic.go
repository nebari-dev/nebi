package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	// UserContextKey is the key used to store user in Gin context
	UserContextKey = "user"
	// TokenDuration is the validity period for JWT tokens
	TokenDuration = 24 * time.Hour
)

type authTokenSource string

const (
	authTokenSourceBasic      authTokenSource = "basic"
	authTokenSourceReconciled authTokenSource = "reconciled"
)

// BasicAuthenticator implements basic username/password authentication
type BasicAuthenticator struct {
	db               *gorm.DB
	jwtSecret        []byte
	proxyAdminGroups []string
	idTokenVerifier  *oidc.IDTokenVerifier
	rbac             rbac.Provider
}

// NewBasicAuthenticator creates a new basic authenticator. The JWT signing
// key is derived from jwtSecret via HKDF (see internal/crypto), independent
// of the key derived from the same secret for registry-credential
// encryption — knowing one does not yield the other.
func NewBasicAuthenticator(db *gorm.DB, jwtSecret string, rbacProvider rbac.Provider) (*BasicAuthenticator, error) {
	signingKey, err := nebicrypto.DeriveSigningKey(jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to derive JWT signing key: %w", err)
	}
	return &BasicAuthenticator{
		db:        db,
		jwtSecret: signingKey,
		rbac:      rbacProvider,
	}, nil
}

// SetProxyAdminGroups configures which IdToken groups grant Nebi admin.
func (a *BasicAuthenticator) SetProxyAdminGroups(groups string) {
	a.proxyAdminGroups = parseAdminGroups(groups)
}

// SetIDTokenVerifier configures the OIDC verifier used to validate IdToken cookies.
func (a *BasicAuthenticator) SetIDTokenVerifier(v *oidc.IDTokenVerifier) {
	a.idTokenVerifier = v
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
	UserID                string           `json:"user_id"` // UUID stored as string
	Username              string           `json:"username"`
	TokenSource           string           `json:"token_source,omitempty"`
	AuthorizationSyncedAt *jwt.NumericDate `json:"authorization_synced_at,omitempty"`
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
	return a.generateTokenWithSource(user, authTokenSourceBasic, nil)
}

func (a *BasicAuthenticator) generateReconciledToken(user *models.User) (string, error) {
	now := time.Now().UTC()
	return a.generateTokenWithAuthorizationSync(user, &now)
}

// generateTokenWithAuthorizationSync keeps the timestamp injectable for tests
// that need to mint stale reconciled tokens. Runtime callers should normally use
// generateReconciledToken so successful reconciliation is stamped with "now".
func (a *BasicAuthenticator) generateTokenWithAuthorizationSync(user *models.User, authorizationSyncedAt *time.Time) (string, error) {
	return a.generateTokenWithSource(user, authTokenSourceReconciled, authorizationSyncedAt)
}

func (a *BasicAuthenticator) generateTokenWithSource(user *models.User, source authTokenSource, authorizationSyncedAt *time.Time) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:      user.ID.String(),
		Username:    user.Username,
		TokenSource: string(source),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "nebi",
		},
	}
	if authorizationSyncedAt != nil {
		claims.AuthorizationSyncedAt = jwt.NewNumericDate(*authorizationSyncedAt)
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
		if err := a.requireFreshAuthorizationSync(claims, time.Now().UTC()); err != nil {
			return nil, err
		}
		return claims, nil
	}

	return nil, ErrUnauthorized
}

func (a *BasicAuthenticator) requireFreshAuthorizationSync(claims *Claims, now time.Time) error {
	staleAfter := authReconciliationStaleAfter()
	err := claims.requireFreshAuthorizationSync(now, staleAfter)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrAuthorizationStale) || claims.AuthorizationSyncedAt == nil {
		return err
	}

	userID, parseErr := uuid.Parse(claims.UserID)
	if parseErr != nil {
		return fmt.Errorf("invalid user ID in token: %w", parseErr)
	}
	fresh, statusErr := hasFreshAuthorizationReconciliationStatus(a.db, userID, claims.AuthorizationSyncedAt.Time, now, staleAfter)
	if statusErr != nil {
		return fmt.Errorf("check authorization reconciliation status: %w", statusErr)
	}
	if fresh {
		return nil
	}
	return err
}

func (c *Claims) requireFreshAuthorizationSync(now time.Time, staleAfter time.Duration) error {
	if c.AuthorizationSyncedAt == nil {
		if c.TokenSource == "" || c.TokenSource == string(authTokenSourceBasic) {
			return nil
		}
		return ErrAuthorizationStale
	}
	if staleAfter <= 0 {
		staleAfter = defaultAuthReconciliationStaleAfter
	}
	if now.After(c.AuthorizationSyncedAt.Time.Add(staleAfter)) {
		return ErrAuthorizationStale
	}
	return nil
}

func hasFreshAuthorizationReconciliationStatus(db *gorm.DB, userID uuid.UUID, tokenSyncedAt time.Time, now time.Time, staleAfter time.Duration) (bool, error) {
	if db == nil {
		return false, nil
	}

	var unresolved int64
	if err := db.Model(&models.AuthReconciliationStatus{}).
		Where("user_id = ?", userID).
		Where("last_failure_at IS NOT NULL").
		Where("last_success_at IS NULL OR last_failure_at > last_success_at").
		Count(&unresolved).Error; err != nil {
		return false, err
	}
	if unresolved > 0 {
		return false, nil
	}

	minSuccessAt := now.Add(-staleAfter)
	if tokenSyncedAt.After(minSuccessAt) {
		minSuccessAt = tokenSyncedAt
	}

	var freshSuccesses int64
	// Tokens do not encode which reconciliation kinds their issuing flow ran,
	// so this is intentionally user-level freshness. Narrow this query if
	// reconciled tokens ever carry kind-level state.
	if err := db.Model(&models.AuthReconciliationStatus{}).
		Where("user_id = ?", userID).
		Where("last_success_at IS NOT NULL").
		Where("last_success_at >= ?", minSuccessAt).
		Count(&freshSuccesses).Error; err != nil {
		return false, err
	}
	return freshSuccesses > 0, nil
}

// Middleware returns a Gin middleware for authentication.
// It checks (in order): Bearer token header, IdToken cookie.
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
		}

		// If we have a Nebi JWT, validate it
		if tokenString != "" {
			user, err := a.validateAndLoadUser(tokenString)
			if err != nil {
				slog.Warn("Invalid token", "error", err)
				if errors.Is(err, ErrAuthorizationStale) {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization reconciliation is stale; re-authentication required"})
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
				}
				c.Abort()
				return
			}
			c.Set(UserContextKey, user)
			c.Next()
			return
		}

		// Fallback: try IdToken cookie from authenticating proxy (e.g. Envoy Gateway)
		proxyClaims, err := verifyIdTokenCookie(c.Request, a.idTokenVerifier)
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

		// Reconcile OIDC group memberships from proxy claim.
		if err := syncOIDCGroups(a.db, user.ID, proxyClaims.Groups, a.rbac); err != nil {
			slog.Error("OIDC group sync failed; rejecting request", "user_id", user.ID, "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization reconciliation failed"})
			c.Abort()
			return
		}

		// Sync admin role from proxy groups on every request
		if err := a.syncProxyAdminRole(user.ID, proxyClaims.Groups, a.proxyAdminGroups); err != nil {
			slog.Error("Proxy admin sync failed; rejecting request", "user_id", user.ID, "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization reconciliation failed"})
			c.Abort()
			return
		}

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
	proxyClaims, err := verifyIdTokenCookie(r, a.idTokenVerifier)
	if err != nil {
		return nil, err
	}

	user, err := findOrCreateProxyUser(a.db, proxyClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to find/create proxy user: %w", err)
	}

	// Reconcile OIDC group memberships from proxy claim.
	if err := syncOIDCGroups(a.db, user.ID, proxyClaims.Groups, a.rbac); err != nil {
		slog.Error("OIDC group sync failed; rejecting session", "user_id", user.ID, "error", err)
		return nil, fmt.Errorf("sync oidc groups: %w", err)
	}

	if err := a.syncProxyAdminRole(user.ID, proxyClaims.Groups, parseAdminGroups(adminGroups)); err != nil {
		slog.Error("Proxy admin sync failed; rejecting session", "user_id", user.ID, "error", err)
		return nil, fmt.Errorf("sync proxy admin role: %w", err)
	}

	token, err := a.generateReconciledToken(user)
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

// ExchangeIDToken verifies a raw OIDC ID token (e.g. from device flow),
// finds/creates the user, syncs roles, and returns a Nebi JWT.
func (a *BasicAuthenticator) ExchangeIDToken(rawIDToken string, adminGroups string) (*LoginResponse, error) {
	if a.idTokenVerifier == nil {
		return nil, errors.New("OIDC verification not configured")
	}

	idToken, err := a.idTokenVerifier.Verify(context.Background(), rawIDToken)
	if err != nil {
		logIdentityProviderAuthFailure(authReconciliationOIDCGroups, err)
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims ProxyTokenClaims
	if err := idToken.Claims(&claims); err != nil {
		logIdentityProviderAuthFailure(authReconciliationOIDCGroups, err)
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	user, err := findOrCreateProxyUser(a.db, &claims)
	if err != nil {
		return nil, fmt.Errorf("failed to find/create user: %w", err)
	}

	if err := syncOIDCGroups(a.db, user.ID, claims.Groups, a.rbac); err != nil {
		slog.Error("OIDC group sync failed; rejecting login", "user_id", user.ID, "error", err)
		return nil, fmt.Errorf("sync oidc groups: %w", err)
	}

	if err := a.syncProxyAdminRole(user.ID, claims.Groups, parseAdminGroups(adminGroups)); err != nil {
		slog.Error("Proxy admin sync failed; rejecting login", "user_id", user.ID, "error", err)
		return nil, fmt.Errorf("sync proxy admin role: %w", err)
	}

	token, err := a.generateReconciledToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResponse{Token: token, User: user}, nil
}
