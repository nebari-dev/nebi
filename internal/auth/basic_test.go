package auth

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestBasicAuthenticator_AcceptsFreshReconciledBearerToken(t *testing.T) {
	db := setupTestDB(t)
	newTestUser(t, db, "alice", "correct-horse-battery-staple")

	authr, err := NewBasicAuthenticator(db, testJWTSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	var user models.User
	if err := db.Where("username = ?", "alice").First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}

	token, err := authr.generateReconciledToken(&user)
	if err != nil {
		t.Fatalf("generate reconciled token: %v", err)
	}

	if code := callWithToken(t, authr.Middleware(), token); code != http.StatusOK {
		t.Fatalf("expected 200 for fresh reconciled token, got %d", code)
	}
}

func TestBasicAuthenticator_RejectsStaleReconciledBearerToken(t *testing.T) {
	db := setupTestDB(t)
	newTestUser(t, db, "alice", "correct-horse-battery-staple")

	authr, err := NewBasicAuthenticator(db, testJWTSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	var user models.User
	if err := db.Where("username = ?", "alice").First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}

	syncedAt := time.Now().UTC().Add(-authReconciliationStaleAfter() - time.Second)
	token, err := authr.generateTokenWithAuthorizationSync(&user, &syncedAt)
	if err != nil {
		t.Fatalf("generate stale reconciled token: %v", err)
	}

	if code := callWithToken(t, authr.Middleware(), token); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for stale reconciled token, got %d", code)
	}
}

func TestBasicAuthenticator_AcceptsLegacyUnstampedBearerToken(t *testing.T) {
	db := setupTestDB(t)
	newTestUser(t, db, "alice", "correct-horse-battery-staple")

	authr, err := NewBasicAuthenticator(db, testJWTSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	var user models.User
	if err := db.Where("username = ?", "alice").First(&user).Error; err != nil {
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
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(authr.jwtSecret)
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}

	if code := callWithToken(t, authr.Middleware(), token); code != http.StatusOK {
		t.Fatalf("expected 200 for legacy unstamped token, got %d", code)
	}
}

func TestBasicAuthenticator_AcceptsStaleReconciledBearerTokenWithFreshStatus(t *testing.T) {
	setAuthReconciliationStaleAfterForTest(t, 5*time.Minute)

	db := setupTestDB(t)
	newTestUser(t, db, "alice", "correct-horse-battery-staple")

	authr, err := NewBasicAuthenticator(db, testJWTSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	var user models.User
	if err := db.Where("username = ?", "alice").First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}

	syncedAt := time.Now().UTC().Add(-10 * time.Minute)
	token, err := authr.generateTokenWithAuthorizationSync(&user, &syncedAt)
	if err != nil {
		t.Fatalf("generate stale reconciled token: %v", err)
	}

	freshSuccess := time.Now().UTC()
	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:        user.ID,
		Kind:          string(authReconciliationOIDCGroups),
		LastSuccessAt: &freshSuccess,
	}).Error; err != nil {
		t.Fatalf("create reconciliation status: %v", err)
	}

	if code := callWithToken(t, authr.Middleware(), token); code != http.StatusOK {
		t.Fatalf("expected 200 for stale token with fresh reconciliation status, got %d", code)
	}
}

func TestBasicAuthenticator_RejectsStaleReconciledBearerTokenAfterCachedOIDCGroupRetry(t *testing.T) {
	setAuthReconciliationStaleAfterForTest(t, 5*time.Minute)

	db := syncTestDB(t)
	newTestUser(t, db, "alice", "correct-horse-battery-staple")

	authr, err := NewBasicAuthenticator(db, testJWTSecret, nil)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	var user models.User
	if err := db.Where("username = ?", "alice").First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}

	syncedAt := time.Now().UTC().Add(-10 * time.Minute)
	token, err := authr.generateTokenWithAuthorizationSync(&user, &syncedAt)
	if err != nil {
		t.Fatalf("generate stale reconciled token: %v", err)
	}

	failureAt := time.Now().UTC()
	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              user.ID,
		Kind:                string(authReconciliationOIDCGroups),
		LastFailureAt:       &failureAt,
		LastFailureSource:   string(authReconciliationFailureSourceLocal),
		ConsecutiveFailures: 1,
		LastError:           "casbin unavailable",
		DesiredGroupsJSON:   encodeOIDCGroupReconciliationState([]string{"engineering"}),
	}).Error; err != nil {
		t.Fatalf("create reconciliation status: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	count, err := alertUnresolvedAuthReconciliations(db, &stubRBACProvider{}, logger, failureAt)
	if err != nil {
		t.Fatalf("alert unresolved reconciliations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected cached group retry to clear failure, got %d unresolved", count)
	}

	if code := callWithToken(t, authr.Middleware(), token); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for stale token after cached group retry, got %d", code)
	}
}

func setAuthReconciliationStaleAfterForTest(t *testing.T, staleAfter time.Duration) {
	t.Helper()
	previous := authReconciliationStaleAfter()
	ConfigureAuthReconciliationStaleAfter(staleAfter)
	t.Cleanup(func() {
		ConfigureAuthReconciliationStaleAfter(previous)
	})
}

func TestBasicAuthenticator_RejectsQueryToken(t *testing.T) {
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

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(authr.Middleware())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/?token="+url.QueryEscape(resp.Token), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for token in query string, got %d", rec.Code)
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
