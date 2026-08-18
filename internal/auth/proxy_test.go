package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Group{},
		&models.GroupMember{},
		&models.AuditLog{},
		&models.AuthReconciliationStatus{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestVerifyIdTokenCookie_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := verifyIdTokenCookie(req, nil)
	if err == nil {
		t.Error("expected error when verifier is nil")
	}
}

func TestVerifyIdTokenCookie_NilVerifierRejects(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "IdToken-nebi", Value: "header.payload.sig"})

	_, err := verifyIdTokenCookie(req, nil)
	if err == nil {
		t.Error("expected error when verifier is nil even with cookie present")
	}
}

func TestFindOrCreateProxyUser_CreatesNew(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		PreferredUsername: "bob",
		Email:             "bob@example.com",
		Picture:           "https://example.com/bob.png",
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Username != "bob" {
		t.Errorf("expected username bob, got %s", user.Username)
	}
	if user.Email != "bob@example.com" {
		t.Errorf("expected email bob@example.com, got %s", user.Email)
	}
	if user.AvatarURL != "https://example.com/bob.png" {
		t.Errorf("expected avatar URL, got %s", user.AvatarURL)
	}

	// Verify in DB
	var count int64
	db.Model(&models.User{}).Where("username = ?", "bob").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 user in db, got %d", count)
	}
}

func TestFindOrCreateProxyUser_FindsExisting(t *testing.T) {
	db := setupTestDB(t)

	existing := models.User{
		ID:           uuid.New(),
		Username:     "carol",
		Email:        "carol@example.com",
		AvatarURL:    "old-avatar",
		PasswordHash: "",
	}
	db.Create(&existing)

	claims := &ProxyTokenClaims{
		PreferredUsername: "carol",
		Email:             "carol@example.com",
		Picture:           "new-avatar",
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != existing.ID {
		t.Error("expected to find existing user, got different ID")
	}
	if user.AvatarURL != "new-avatar" {
		t.Errorf("expected avatar to be updated to new-avatar, got %s", user.AvatarURL)
	}
}

func TestFindOrCreateProxyUser_FallbackToEmail(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		Email: "dave@example.com",
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Username != "dave@example.com" {
		t.Errorf("expected username to fall back to email, got %s", user.Username)
	}
}

func TestFindOrCreateProxyUser_FallbackToSub(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		Sub: "sub-xyz",
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Username != "sub-xyz" {
		t.Errorf("expected username to fall back to sub, got %s", user.Username)
	}
}

func TestFindOrCreateProxyUser_NoIdentity(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{}
	_, err := findOrCreateProxyUser(db, claims)
	if err == nil {
		t.Error("expected error when no identity claim present")
	}
}

func TestParseAdminGroups(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"admin", []string{"admin"}},
		{"admin,nebi-admin", []string{"admin", "nebi-admin"}},
		{" admin , nebi-admin , ", []string{"admin", "nebi-admin"}},
	}

	for _, tt := range tests {
		got := parseAdminGroups(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseAdminGroups(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseAdminGroups(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestShouldBeAdminFromGroups_MatchesWithSlash(t *testing.T) {
	provider := &stubRBACProvider{}
	shouldBeAdmin := shouldBeAdminFromGroups([]string{"/admin", "/dev"}, []string{"admin"})

	if err := syncAdminRoleToDesired(uuid.New(), shouldBeAdmin, provider); err != nil {
		t.Fatalf("sync roles: %v", err)
	}
	if !provider.madeAdmin {
		t.Fatal("expected admin grant for slash-prefixed admin group")
	}
}

func TestSyncAdminRoleToDesired_ReturnsAdminCheckFailure(t *testing.T) {
	wantErr := errors.New("casbin unavailable")
	provider := &stubRBACProvider{isAdminErr: wantErr}

	err := syncAdminRoleToDesired(uuid.New(), true, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected admin check error, got %v", err)
	}
	if provider.madeAdmin || provider.revokedAdmin {
		t.Fatal("expected no admin mutation when status check fails")
	}
}

func TestSyncAdminRoleToDesired_ReturnsGrantFailure(t *testing.T) {
	wantErr := errors.New("grant failed")
	provider := &stubRBACProvider{makeAdminErr: wantErr}

	err := syncAdminRoleToDesired(uuid.New(), true, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected grant error, got %v", err)
	}
	if !provider.madeAdmin {
		t.Fatal("expected admin grant to be attempted")
	}
}

func TestSyncAdminRoleToDesired_ReturnsRevokeFailure(t *testing.T) {
	wantErr := errors.New("revoke failed")
	provider := &stubRBACProvider{isAdmin: true, revokeAdminErr: wantErr}

	err := syncAdminRoleToDesired(uuid.New(), false, provider)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected revoke error, got %v", err)
	}
	if !provider.revokedAdmin {
		t.Fatal("expected admin revoke to be attempted")
	}
}

func TestSyncProxyAdminRole_ReturnsStatusCreateFailure(t *testing.T) {
	db := setupTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	provider := &stubRBACProvider{}

	authr, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}

	wantErr := errors.New("status create failed")
	name := registerDBTableFailureCallback(t, db, "create", "auth_reconciliation_statuses", wantErr)
	defer db.Callback().Create().Remove(name)

	err = authr.syncProxyAdminRole(u.ID, []string{"admin"}, []string{"admin"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected status create error, got %v", err)
	}
	if !provider.madeAdmin {
		t.Fatal("expected admin grant to be attempted before status write failed")
	}
}

func TestSyncProxyAdminRole_ReturnsStatusUpdateFailure(t *testing.T) {
	db := setupTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	provider := &stubRBACProvider{}

	authr, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	if err := authr.syncProxyAdminRole(u.ID, nil, []string{"admin"}); err != nil {
		t.Fatalf("seed status: %v", err)
	}
	oldSuccess := time.Now().UTC().Add(-authReconciliationSuccessRefreshAfter() - time.Second)
	if err := db.Model(&models.AuthReconciliationStatus{}).
		Where("user_id = ? AND kind = ?", u.ID, string(authReconciliationProxyAdmin)).
		Update("last_success_at", oldSuccess).Error; err != nil {
		t.Fatalf("age status: %v", err)
	}

	wantErr := errors.New("status update failed")
	name := registerDBTableFailureCallback(t, db, "update", "auth_reconciliation_statuses", wantErr)
	defer db.Callback().Update().Remove(name)

	err = authr.syncProxyAdminRole(u.ID, nil, []string{"admin"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected status update error, got %v", err)
	}
}

func TestSessionFromProxy_DoesNotMintTokenWhenAdminRevokeFails(t *testing.T) {
	db := setupTestDB(t)
	wantErr := errors.New("revoke failed")
	provider := &stubRBACProvider{isAdmin: true, revokeAdminErr: wantErr}

	authr, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authr.SetIDTokenVerifier(testProxyVerifier())

	resp, err := authr.SessionFromProxy(testProxyRequest(t, "alice", nil), "admin")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected revoke error, got %v", err)
	}
	if resp != nil {
		t.Fatal("expected no session response when admin revoke fails")
	}
}

func TestSessionFromProxy_MintsReconciledToken(t *testing.T) {
	db := setupTestDB(t)
	provider := &stubRBACProvider{}

	authr, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authr.SetIDTokenVerifier(testProxyVerifier())

	resp, err := authr.SessionFromProxy(testProxyRequest(t, "alice", []string{"engineering"}), "admin")
	if err != nil {
		t.Fatalf("SessionFromProxy: %v", err)
	}

	claims, err := authr.validateToken(resp.Token)
	if err != nil {
		t.Fatalf("validate reconciled token: %v", err)
	}
	if claims.AuthorizationSyncedAt == nil {
		t.Fatal("expected reconciled proxy session token to carry authorization sync timestamp")
	}
}

func TestExchangeIDToken_DoesNotMintTokenWhenGroupSyncFails(t *testing.T) {
	db := setupTestDB(t)
	wantErr := errors.New("casbin list failed")
	provider := &stubRBACProvider{getUserGroupsErr: wantErr}

	authr, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authr.SetIDTokenVerifier(testProxyVerifier())

	resp, err := authr.ExchangeIDToken(testIDToken(t, "alice", []string{"engineering"}), "admin")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected group sync error, got %v", err)
	}
	if resp != nil {
		t.Fatal("expected no device token response when group sync fails")
	}
}

func TestExchangeIDToken_DoesNotMintTokenWhenAdminRevokeFails(t *testing.T) {
	db := setupTestDB(t)
	wantErr := errors.New("revoke failed")
	provider := &stubRBACProvider{isAdmin: true, revokeAdminErr: wantErr}

	authr, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authr.SetIDTokenVerifier(testProxyVerifier())

	resp, err := authr.ExchangeIDToken(testIDToken(t, "alice", nil), "admin")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected revoke error, got %v", err)
	}
	if resp != nil {
		t.Fatal("expected no device token response when admin revoke fails")
	}
}

func TestMiddlewareRejectsProxyRequestWhenAdminRevokeFails(t *testing.T) {
	db := setupTestDB(t)
	provider := &stubRBACProvider{isAdmin: true, revokeAdminErr: errors.New("revoke failed")}

	authr, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authr.SetIDTokenVerifier(testProxyVerifier())

	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	router.Use(authr.Middleware())
	router.GET("/", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, testProxyRequest(t, "alice", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when admin revoke fails, got %d", rec.Code)
	}
	if called {
		t.Fatal("expected protected handler not to run")
	}
}

func testProxyVerifier() *oidc.IDTokenVerifier {
	return oidc.NewVerifier("", nil, &oidc.Config{
		SkipClientIDCheck:          true,
		SkipExpiryCheck:            true,
		SkipIssuerCheck:            true,
		InsecureSkipSignatureCheck: true,
	})
}

func testProxyRequest(t *testing.T, username string, groups []string) *http.Request {
	t.Helper()
	rawToken := testIDToken(t, username, groups)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "IdToken-nebi", Value: rawToken})
	return req
}

func testIDToken(t *testing.T, username string, groups []string) string {
	t.Helper()
	// alg=none is deliberate: testProxyVerifier skips signature checks, so
	// these tests only need claim payloads, not cryptographic signatures.
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":                username,
		"preferred_username": username,
		"email":              username + "@example.com",
		"groups":             groups,
	})
	rawToken, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign test id token: %v", err)
	}
	return rawToken
}
