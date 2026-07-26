package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

const testJWTSecret = "a-sufficiently-long-test-secret-value"

func newTestUser(t *testing.T, db *gorm.DB, username, password string) {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := models.User{Username: username, Email: username + "@example.com", PasswordHash: hash}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func callWithToken(t *testing.T, mw gin.HandlerFunc, token string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(mw)
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestNewBasicAuthenticator_RejectsEmptySecret(t *testing.T) {
	db := setupTestDB(t)
	_, err := NewBasicAuthenticator(db, "", nil)
	if err == nil {
		t.Fatal("expected error for empty JWT secret")
	}
}

func TestBasicAuthenticator_LoginTokenIsAccepted(t *testing.T) {
	db := setupTestDB(t)
	newTestUser(t, db, "alice", "correct-horse-battery-staple")

	authr, err := NewBasicAuthenticator(db, testJWTSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	resp, err := authr.Login("alice", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if code := callWithToken(t, authr.Middleware(), resp.Token); code != http.StatusOK {
		t.Fatalf("expected 200 for a token signed by Login, got %d", code)
	}
}

// TestBasicAuthenticator_RejectsTokenForgedWithRawSecret is the regression
// test for issue #443: the signing key must be derived (HKDF) from the
// configured secret, not the raw secret bytes. A token forged by an attacker
// who knows the raw secret but signs it directly (the pre-fix behavior) must
// be rejected.
func TestBasicAuthenticator_RejectsTokenForgedWithRawSecret(t *testing.T) {
	db := setupTestDB(t)
	newTestUser(t, db, "bob", "correct-horse-battery-staple")

	authr, err := NewBasicAuthenticator(db, testJWTSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	var user models.User
	if err := db.Where("username = ?", "bob").First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}

	claims := Claims{
		UserID:   user.ID.String(),
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "nebi",
		},
	}
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	forgedToken, err := forged.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign forged token: %v", err)
	}

	if code := callWithToken(t, authr.Middleware(), forgedToken); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for token forged with raw secret, got %d", code)
	}
}
