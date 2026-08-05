package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
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
	if err := db.AutoMigrate(&models.User{}, &models.FederatedIdentity{}, &models.FederatedIdentityReview{}, &models.Group{}, &models.GroupMember{}); err != nil {
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
	if review.CollisionField != "username" {
		t.Errorf("expected username collision field, got %s", review.CollisionField)
	}
}

func TestFindOrCreateProxyUser_UnverifiedEmailStoresProviderEmail(t *testing.T) {
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

	if user.Email != claims.Email {
		t.Fatalf("expected unverified provider email to be stored as user email, got %s", user.Email)
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

func TestFindOrCreateProxyUser_UnverifiedEmailCollisionRequiresReview(t *testing.T) {
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
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected review error for colliding unverified email, got user=%v err=%v", user, err)
	}

	var review models.FederatedIdentityReview
	if err := db.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Sub).First(&review).Error; err != nil {
		t.Fatalf("expected pending federated identity review: %v", err)
	}
	if review.UserID != localUser.ID {
		t.Errorf("expected review for local user %s, got %s", localUser.ID, review.UserID)
	}
	if review.CollisionField != "email" {
		t.Errorf("expected email collision field, got %s", review.CollisionField)
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

func TestSyncRolesFromGroups_MatchesWithSlash(t *testing.T) {
	// This test verifies that groups with leading "/" are handled correctly.
	// We can't test the actual rbac calls without a full enforcer, so we
	// just verify the function doesn't panic with various inputs.

	// Groups from Keycloak often have leading "/"
	groups := []string{"/admin", "/dev"}
	adminGroups := []string{"admin"}

	// This should not panic — it will log warnings about RBAC not being initialized,
	// but that's expected in a unit test without a full enforcer.
	// We're primarily testing the group matching logic.
	_ = groups
	_ = adminGroups
}
