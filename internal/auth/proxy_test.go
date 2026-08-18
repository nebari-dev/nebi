package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		&models.FederatedIdentity{},
		&models.FederatedIdentityReview{},
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
		Issuer:            "https://issuer.example.com",
		Sub:               "sub-bob",
		PreferredUsername: "bob",
		Email:             "bob@example.com",
		EmailVerified:     true,
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

	db.Model(&models.FederatedIdentity{}).Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 federated identity in db, got %d", count)
	}
}

func TestFindOrCreateProxyUser_FindsExisting(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "sub-carol",
		PreferredUsername: "carol",
		Email:             "carol@example.com",
		EmailVerified:     true,
		Picture:           "old-avatar",
	}
	existing, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	changedClaims := &ProxyTokenClaims{
		Issuer:            claims.Issuer,
		Sub:               claims.Sub,
		PreferredUsername: "changed-carol",
		Email:             "changed-carol@example.com",
		EmailVerified:     true,
		Picture:           "new-avatar",
	}

	user, err := findOrCreateProxyUser(db, changedClaims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != existing.ID {
		t.Error("expected to find existing user, got different ID")
	}
	if user.Username != "carol" {
		t.Errorf("expected bound user's username to remain carol, got %s", user.Username)
	}
	if user.AvatarURL != "new-avatar" {
		t.Errorf("expected avatar to be updated to new-avatar, got %s", user.AvatarURL)
	}
}

func TestFindOrCreateProxyUser_FallbackToEmail(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		Issuer:        "https://issuer.example.com",
		Sub:           "sub-dave",
		Email:         "dave@example.com",
		EmailVerified: true,
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Username != "dave@example.com" {
		t.Errorf("expected username to fall back to email, got %s", user.Username)
	}
	if user.Email != "dave@example.com" {
		t.Errorf("expected user email to come from provider email, got %s", user.Email)
	}
}

func TestFindOrCreateProxyUser_UniquifiesOnlyFallbackUsername(t *testing.T) {
	db := setupTestDB(t)

	existing := models.User{
		ID:           uuid.New(),
		Username:     "dave@example.com",
		Email:        "local-dave@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	claims := &ProxyTokenClaims{
		Issuer:        "https://issuer.example.com",
		Sub:           "sub-dave",
		Email:         "dave@example.com",
		EmailVerified: true,
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Username == claims.Email {
		t.Fatal("expected fallback username to be uniquified")
	}
	if !strings.HasPrefix(user.Username, claims.Email+"-") {
		t.Errorf("expected fallback username to be based on provider email, got %s", user.Username)
	}
	if user.Email != claims.Email {
		t.Errorf("expected user email to come from provider email, got %s", user.Email)
	}
}

func TestFindOrCreateProxyUser_FallbackToSub(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		Issuer: "https://issuer.example.com",
		Sub:    "sub-xyz",
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Username != "sub-xyz" {
		t.Errorf("expected username to fall back to sub, got %s", user.Username)
	}
	if user.Email != "sub-xyz@nebi.local" {
		t.Errorf("expected missing email to fall back to sub@nebi.local, got %s", user.Email)
	}
}

func TestFindOrCreateProxyUser_MissingEmailFallsBackToUsername(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "sub-frank",
		PreferredUsername: "frank",
	}

	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Email != "frank@nebi.local" {
		t.Errorf("expected missing email to fall back to username@nebi.local, got %s", user.Email)
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

func TestIsUniqueConstraintErrorOnlyMatchesUniqueConstraints(t *testing.T) {
	db := setupTestDB(t)
	existing := models.User{
		ID:           uuid.New(),
		Username:     "unique-test",
		Email:        "unique@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	duplicate := models.User{
		ID:           uuid.New(),
		Username:     existing.Username,
		Email:        "duplicate@example.com",
		PasswordHash: "hashed-password",
	}
	uniqueErr := db.Create(&duplicate).Error
	if uniqueErr == nil {
		t.Fatal("expected duplicate username error")
	}
	if !isUniqueConstraintError(uniqueErr) {
		t.Fatalf("expected duplicate username to be treated as unique constraint, got %v", uniqueErr)
	}

	notNullErr := db.Exec(
		"INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		uuid.New().String(),
		"missing-email",
		"hashed-password",
		time.Now().UTC(),
		time.Now().UTC(),
	).Error
	if notNullErr == nil {
		t.Fatal("expected missing email to violate NOT NULL")
	}
	if isUniqueConstraintError(notNullErr) {
		t.Fatalf("expected NOT NULL constraint not to be treated as unique, got %v", notNullErr)
	}
}

func TestFindOrCreateProxyUser_DoesNotLinkByUsernameOrEmail(t *testing.T) {
	db := setupTestDB(t)

	localUser := models.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}

	claims := &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "attacker-sub",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		EmailVerified:     true,
	}
	user, err := findOrCreateProxyUser(db, claims)
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected review error for colliding unlinked user, got user=%v err=%v", user, err)
	}

	var review models.FederatedIdentityReview
	if err := db.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).First(&review).Error; err != nil {
		t.Fatalf("expected pending federated identity review: %v", err)
	}
	if review.UserID != localUser.ID {
		t.Errorf("expected review for local user %s, got %s", localUser.ID, review.UserID)
	}
	if review.CollisionField != models.FederatedIdentityReviewCollisionUsernameEmail {
		t.Errorf("expected username+email collision field, got %s", review.CollisionField)
	}
}

func TestFindOrCreateProxyUser_PendingReviewTargetDoesNotMove(t *testing.T) {
	db := setupTestDB(t)

	alice := models.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed-password",
	}
	admin := models.User{
		ID:           uuid.New(),
		Username:     "root-admin",
		Email:        "admin@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&alice).Error; err != nil {
		t.Fatalf("create alice user: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	claims := &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "stable-sub",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		EmailVerified:     true,
	}
	user, err := findOrCreateProxyUser(db, claims)
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected initial review error, got user=%v err=%v", user, err)
	}

	var review models.FederatedIdentityReview
	if err := db.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).First(&review).Error; err != nil {
		t.Fatalf("expected pending federated identity review: %v", err)
	}

	changedClaims := &ProxyTokenClaims{
		Issuer:            claims.Issuer,
		Sub:               claims.Sub,
		PreferredUsername: "root-admin",
		Email:             "admin@example.com",
		EmailVerified:     true,
	}
	user, err = findOrCreateProxyUser(db, changedClaims)
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected repeated review error, got user=%v err=%v", user, err)
	}

	var reviewCount int64
	db.Model(&models.FederatedIdentityReview{}).Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).Count(&reviewCount)
	if reviewCount != 1 {
		t.Fatalf("expected exactly one review row, got %d", reviewCount)
	}

	var persisted models.FederatedIdentityReview
	if err := db.First(&persisted, "id = ?", review.ID).Error; err != nil {
		t.Fatalf("load persisted review: %v", err)
	}
	if persisted.UserID != alice.ID {
		t.Errorf("expected review to remain bound to alice %s, got %s", alice.ID, persisted.UserID)
	}
	if persisted.Username != "alice" || persisted.Email != "alice@example.com" {
		t.Errorf("expected original claims to remain, got username=%q email=%q", persisted.Username, persisted.Email)
	}
}

func TestFindOrCreateProxyUser_UnverifiedEmailUsesSyntheticUserEmail(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "unverified-sub",
		PreferredUsername: "erin",
		Email:             "erin@example.com",
		EmailVerified:     false,
	}
	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Email != "erin@nebi.local" {
		t.Fatalf("expected unverified provider email not to be stored as user email, got %s", user.Email)
	}

	var identity models.FederatedIdentity
	if err := db.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).First(&identity).Error; err != nil {
		t.Fatalf("load federated identity: %v", err)
	}
	if identity.Email != claims.Email {
		t.Errorf("expected profile email to be retained, got %s", identity.Email)
	}
	if identity.EmailVerified {
		t.Fatal("expected email_verified=false to be retained")
	}
}

func TestFindOrCreateProxyUser_UnverifiedEmailCollisionDoesNotRequireReview(t *testing.T) {
	db := setupTestDB(t)

	localUser := models.User{
		ID:           uuid.New(),
		Username:     "local-erin",
		Email:        "erin@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}

	claims := &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "unverified-local-email-sub",
		PreferredUsername: "remote-erin",
		Email:             "erin@example.com",
		EmailVerified:     false,
	}
	user, err := findOrCreateProxyUser(db, claims)
	if err != nil {
		t.Fatalf("unexpected error for unverified email collision: %v", err)
	}
	if user.ID == localUser.ID {
		t.Fatal("expected unverified email not to bind to the existing local user")
	}
	if user.Email != "remote-erin@nebi.local" {
		t.Fatalf("expected synthetic user email for unverified provider email, got %s", user.Email)
	}

	var reviewCount int64
	db.Model(&models.FederatedIdentityReview{}).Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).Count(&reviewCount)
	if reviewCount != 0 {
		t.Fatalf("expected no review for unverified email-only collision, got %d", reviewCount)
	}

	var identity models.FederatedIdentity
	if err := db.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).First(&identity).Error; err != nil {
		t.Fatalf("load federated identity: %v", err)
	}
	if identity.Email != claims.Email {
		t.Errorf("expected federated profile email to retain provider email, got %s", identity.Email)
	}
	if identity.EmailVerified {
		t.Fatal("expected email_verified=false to be retained")
	}
}

func TestFindOrCreateProxyUser_AmbiguousUsernameEmailCollisionRequiresReview(t *testing.T) {
	db := setupTestDB(t)

	alice := models.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed-password",
	}
	bob := models.User{
		ID:           uuid.New(),
		Username:     "bob",
		Email:        "bob@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&alice).Error; err != nil {
		t.Fatalf("create alice user: %v", err)
	}
	if err := db.Create(&bob).Error; err != nil {
		t.Fatalf("create bob user: %v", err)
	}

	claims := &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "ambiguous-sub",
		PreferredUsername: "alice",
		Email:             "bob@example.com",
		EmailVerified:     true,
	}
	user, err := findOrCreateProxyUser(db, claims)
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected review error for ambiguous collision, got user=%v err=%v", user, err)
	}

	var review models.FederatedIdentityReview
	if err := db.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).First(&review).Error; err != nil {
		t.Fatalf("expected pending federated identity review: %v", err)
	}
	if review.UserID != alice.ID {
		t.Errorf("expected review to target username collision user %s, got %s", alice.ID, review.UserID)
	}
	if review.CollisionField != models.FederatedIdentityReviewCollisionUsernameEmail {
		t.Errorf("expected username+email collision field, got %s", review.CollisionField)
	}
	if review.CollisionUsernameUserID == nil || *review.CollisionUsernameUserID != alice.ID {
		t.Fatalf("expected username collision user %s, got %v", alice.ID, review.CollisionUsernameUserID)
	}
	if review.CollisionEmailUserID == nil || *review.CollisionEmailUserID != bob.ID {
		t.Fatalf("expected email collision user %s, got %v", bob.ID, review.CollisionEmailUserID)
	}
	if !review.HasAmbiguousCollision() {
		t.Fatal("expected review to be marked ambiguous")
	}
}

func TestFindOrCreateProxyUser_DistinguishesIssuersWithSameSubject(t *testing.T) {
	db := setupTestDB(t)

	first, err := findOrCreateProxyUser(db, &ProxyTokenClaims{
		Issuer:            "https://issuer-a.example.com",
		Sub:               "shared-sub",
		PreferredUsername: "shared-a",
		Email:             "shared-a@example.com",
		EmailVerified:     true,
	})
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}

	second, err := findOrCreateProxyUser(db, &ProxyTokenClaims{
		Issuer:            "https://issuer-b.example.com",
		Sub:               "shared-sub",
		PreferredUsername: "shared-b",
		Email:             "shared-b@example.com",
		EmailVerified:     true,
	})
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}

	if second.ID == first.ID {
		t.Fatal("expected same subject from different issuers to remain distinct")
	}

	var count int64
	db.Model(&models.FederatedIdentity{}).Count(&count)
	if count != 2 {
		t.Errorf("expected two federated identities, got %d", count)
	}
}

func TestFindOrCreateProxyUser_RecycledClaimsRequireReview(t *testing.T) {
	db := setupTestDB(t)

	first, err := findOrCreateProxyUser(db, &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "old-sub",
		PreferredUsername: "recycled",
		Email:             "recycled@example.com",
		EmailVerified:     true,
	})
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}

	second, err := findOrCreateProxyUser(db, &ProxyTokenClaims{
		Issuer:            "https://issuer.example.com",
		Sub:               "new-sub",
		PreferredUsername: "recycled",
		Email:             "recycled@example.com",
		EmailVerified:     true,
	})
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected review error for recycled claims, got user=%v err=%v", second, err)
	}

	var review models.FederatedIdentityReview
	if err := db.Where("issuer = ? AND subject = ?", "https://issuer.example.com", "new-sub").First(&review).Error; err != nil {
		t.Fatalf("expected pending federated identity review: %v", err)
	}
	if review.UserID != first.ID {
		t.Errorf("expected review for existing federated user %s, got %s", first.ID, review.UserID)
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
		"iss":                "https://issuer.example.com",
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
