package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

// newDiscoveryServer starts a test server that serves an OIDC discovery
// document advertising `issuer` regardless of the URL used to reach it. This
// mimics a Keycloak that is reachable in-cluster at one URL while its tokens
// carry the external (public) issuer.
func newDiscoveryServer(t *testing.T, issuer string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/protocol/openid-connect/auth",
			"token_endpoint":                        srv.URL + "/protocol/openid-connect/token",
			"jwks_uri":                              srv.URL + "/protocol/openid-connect/certs",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	})
	t.Cleanup(srv.Close)
	return srv
}

// TestNewOIDCAuthenticator_DiscoveryURLAllowsIssuerMismatch verifies the
// split-horizon case: discovery is fetched from an in-cluster URL while the
// document advertises a different (public) issuer. Setting DiscoveryURL must
// make this succeed; the issuer is still kept for "iss" validation.
func TestNewOIDCAuthenticator_DiscoveryURLAllowsIssuerMismatch(t *testing.T) {
	const publicIssuer = "https://keycloak.example.com/realms/nebari"
	srv := newDiscoveryServer(t, publicIssuer)

	cfg := OIDCConfig{
		IssuerURL:    publicIssuer,
		DiscoveryURL: srv.URL,
		ClientID:     "nebi",
	}
	if _, err := NewOIDCAuthenticator(context.Background(), cfg, nil, "test-secret", nil); err != nil {
		t.Fatalf("expected discovery via DiscoveryURL to succeed, got error: %v", err)
	}
}

// TestNewOIDCAuthenticator_IssuerMismatchWithoutDiscoveryURLFails verifies the
// default behavior is unchanged: with no DiscoveryURL, discovery is fetched
// from IssuerURL and go-oidc still rejects an "iss" that does not match the
// URL it was fetched from.
func TestNewOIDCAuthenticator_IssuerMismatchWithoutDiscoveryURLFails(t *testing.T) {
	const publicIssuer = "https://keycloak.example.com/realms/nebari"
	srv := newDiscoveryServer(t, publicIssuer)

	cfg := OIDCConfig{
		IssuerURL: srv.URL, // document advertises publicIssuer, not srv.URL
		ClientID:  "nebi",
	}
	if _, err := NewOIDCAuthenticator(context.Background(), cfg, nil, "test-secret", nil); err == nil {
		t.Fatal("expected issuer mismatch to fail without DiscoveryURL, got nil error")
	}
}

func TestOIDCFindOrCreateUser_DoesNotLinkByEmail(t *testing.T) {
	db := setupTestDB(t)
	existing := models.User{
		ID:           uuid.New(),
		Username:     "local-alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	authenticator := &OIDCAuthenticator{db: db}
	user, err := authenticator.findOrCreateUser(federatedUserClaims{
		Issuer:            "https://issuer.example.com",
		Subject:           "oidc-alice",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		EmailVerified:     true,
	})
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected review error for colliding unlinked user, got user=%v err=%v", user, err)
	}

	var review models.FederatedIdentityReview
	if err := db.Where("issuer = ? AND subject = ?", "https://issuer.example.com", "oidc-alice").First(&review).Error; err != nil {
		t.Fatalf("expected pending federated identity review: %v", err)
	}
	if review.UserID != existing.ID {
		t.Errorf("expected review for local user %s, got %s", existing.ID, review.UserID)
	}
}

func TestOIDCFindOrCreateUser_CaseInsensitiveCollisionRequiresReview(t *testing.T) {
	db := setupTestDB(t)
	existing := models.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	authenticator := &OIDCAuthenticator{db: db}
	user, err := authenticator.findOrCreateUser(federatedUserClaims{
		Issuer:            "https://issuer.example.com",
		Subject:           "oidc-alice",
		PreferredUsername: "ALICE",
		Email:             "ALICE@EXAMPLE.COM",
		EmailVerified:     true,
	})
	if !errors.Is(err, errFederatedIdentityRequiresReview) {
		t.Fatalf("expected review error for case-insensitive collision, got user=%v err=%v", user, err)
	}

	var review models.FederatedIdentityReview
	if err := db.Where("issuer = ? AND subject = ?", "https://issuer.example.com", "oidc-alice").First(&review).Error; err != nil {
		t.Fatalf("expected pending federated identity review: %v", err)
	}
	if review.UserID != existing.ID {
		t.Errorf("expected review for local user %s, got %s", existing.ID, review.UserID)
	}
	if review.CollisionField != models.FederatedIdentityReviewCollisionUsernameEmail {
		t.Errorf("expected username+email collision field, got %s", review.CollisionField)
	}
}

func TestOIDCFindOrCreateUser_RejectedReviewBlocksRepeatLogin(t *testing.T) {
	db := setupTestDB(t)
	existing := models.User{
		ID:           uuid.New(),
		Username:     "local-alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	review := models.FederatedIdentityReview{
		UserID:         existing.ID,
		Issuer:         "https://issuer.example.com",
		Subject:        "oidc-alice",
		CollisionField: "email",
		Username:       "alice",
		Email:          "alice@example.com",
		EmailVerified:  true,
		Status:         models.FederatedIdentityReviewStatusRejected,
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create rejected review: %v", err)
	}

	authenticator := &OIDCAuthenticator{db: db}
	user, err := authenticator.findOrCreateUser(federatedUserClaims{
		Issuer:            "https://issuer.example.com",
		Subject:           "oidc-alice",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		EmailVerified:     true,
	})
	if !errors.Is(err, errFederatedIdentityRejected) {
		t.Fatalf("expected rejected review error, got user=%v err=%v", user, err)
	}

	var persisted models.FederatedIdentityReview
	if err := db.First(&persisted, "id = ?", review.ID).Error; err != nil {
		t.Fatalf("load persisted review: %v", err)
	}
	if persisted.Status != models.FederatedIdentityReviewStatusRejected {
		t.Errorf("expected review to remain rejected, got %q", persisted.Status)
	}
}

func TestOIDCFindOrCreateUser_RejectedReviewBlocksWithoutCurrentCollision(t *testing.T) {
	db := setupTestDB(t)
	existing := models.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	review := models.FederatedIdentityReview{
		UserID:         existing.ID,
		Issuer:         "https://issuer.example.com",
		Subject:        "oidc-alice",
		CollisionField: "email",
		Username:       "alice",
		Email:          "alice@example.com",
		EmailVerified:  true,
		Status:         models.FederatedIdentityReviewStatusRejected,
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create rejected review: %v", err)
	}
	if err := db.Model(&existing).Updates(map[string]any{
		"username": "renamed-alice",
		"email":    "renamed-alice@example.com",
	}).Error; err != nil {
		t.Fatalf("rename existing user: %v", err)
	}

	authenticator := &OIDCAuthenticator{db: db}
	user, err := authenticator.findOrCreateUser(federatedUserClaims{
		Issuer:            "https://issuer.example.com",
		Subject:           "oidc-alice",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		EmailVerified:     true,
	})
	if !errors.Is(err, errFederatedIdentityRejected) {
		t.Fatalf("expected rejected review error without current collision, got user=%v err=%v", user, err)
	}
}

func TestOIDCFindOrCreateUser_RemovesOrphanedIdentityAndReprovisions(t *testing.T) {
	db := setupTestDB(t)
	existing := models.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	if err := db.Create(&models.FederatedIdentity{
		UserID:        existing.ID,
		Issuer:        "https://issuer.example.com",
		Subject:       "oidc-alice",
		Username:      "alice",
		Email:         "alice@example.com",
		EmailVerified: true,
	}).Error; err != nil {
		t.Fatalf("create federated identity: %v", err)
	}
	if err := db.Create(&models.FederatedIdentityReview{
		UserID:         existing.ID,
		Issuer:         "https://issuer.example.com",
		Subject:        "oidc-alice",
		CollisionField: "email",
		Username:       "alice",
		Email:          "alice@example.com",
		EmailVerified:  true,
	}).Error; err != nil {
		t.Fatalf("create stale federated identity review: %v", err)
	}
	if err := db.Delete(&existing).Error; err != nil {
		t.Fatalf("soft-delete existing user: %v", err)
	}

	authenticator := &OIDCAuthenticator{db: db}
	user, err := authenticator.findOrCreateUser(federatedUserClaims{
		Issuer:            "https://issuer.example.com",
		Subject:           "oidc-alice",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		EmailVerified:     true,
	})
	if err != nil {
		t.Fatalf("expected reprovisioned user, got error: %v", err)
	}
	if user.ID == existing.ID {
		t.Fatal("expected a new active user")
	}
	if !strings.HasPrefix(user.Username, "alice-") {
		t.Fatalf("expected username to avoid soft-deleted row, got %s", user.Username)
	}
	if !strings.HasPrefix(user.Email, "alice+") {
		t.Fatalf("expected email to avoid soft-deleted row, got %s", user.Email)
	}

	var identity models.FederatedIdentity
	if err := db.Where("issuer = ? AND subject = ?", "https://issuer.example.com", "oidc-alice").First(&identity).Error; err != nil {
		t.Fatalf("load new federated identity: %v", err)
	}
	if identity.UserID != user.ID {
		t.Fatalf("expected identity user %s, got %s", user.ID, identity.UserID)
	}

	var reviewCount int64
	db.Unscoped().Model(&models.FederatedIdentityReview{}).Where("issuer = ? AND subject = ?", "https://issuer.example.com", "oidc-alice").Count(&reviewCount)
	if reviewCount != 0 {
		t.Fatalf("expected stale review to be hard-deleted, got %d", reviewCount)
	}
}

func TestOIDCFindOrCreateUser_UnchangedProfileDoesNotWriteIdentity(t *testing.T) {
	db := setupTestDB(t)
	user := models.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		AvatarURL:    "https://example.com/alice.png",
		PasswordHash: "",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	updatedAt := time.Now().UTC().Add(-time.Hour)
	identity := models.FederatedIdentity{
		UserID:        user.ID,
		Issuer:        "https://issuer.example.com",
		Subject:       "oidc-alice",
		Username:      "alice",
		Email:         "alice@example.com",
		EmailVerified: true,
		Name:          "Alice",
		AvatarURL:     "https://example.com/alice.png",
		UpdatedAt:     updatedAt,
	}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatalf("create federated identity: %v", err)
	}
	if err := db.Model(&identity).Update("updated_at", updatedAt).Error; err != nil {
		t.Fatalf("set identity updated_at: %v", err)
	}

	authenticator := &OIDCAuthenticator{db: db}
	if _, err := authenticator.findOrCreateUser(federatedUserClaims{
		Issuer:            "https://issuer.example.com",
		Subject:           "oidc-alice",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		EmailVerified:     true,
		Name:              "Alice",
		AvatarURL:         "https://example.com/alice.png",
	}); err != nil {
		t.Fatalf("find existing user: %v", err)
	}

	var persisted models.FederatedIdentity
	if err := db.First(&persisted, "id = ?", identity.ID).Error; err != nil {
		t.Fatalf("load identity: %v", err)
	}
	if persisted.UpdatedAt.After(updatedAt.Add(time.Second)) {
		t.Fatalf("expected unchanged profile not to update updated_at, got %s", persisted.UpdatedAt)
	}
}

func TestOIDCAuthenticator_DoesNotMintTokenWhenGroupSyncFails(t *testing.T) {
	db := setupTestDB(t)
	wantErr := errors.New("casbin list failed")
	provider := &stubRBACProvider{getUserGroupsErr: wantErr}

	basicAuth, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authr := &OIDCAuthenticator{
		db:        db,
		basicAuth: basicAuth,
		rbac:      provider,
	}

	resp, err := authr.loginWithVerifiedClaims("https://issuer.example.com", "", oidcLoginClaims{
		Email:  "alice@example.com",
		Sub:    "alice-subject",
		Groups: []string{"engineering"},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected group sync error, got %v", err)
	}
	if resp != nil {
		t.Fatal("expected no OIDC login response when group sync fails")
	}
}

func TestOIDCAuthenticator_MintsReconciledToken(t *testing.T) {
	db := setupTestDB(t)
	provider := &stubRBACProvider{}

	basicAuth, err := NewBasicAuthenticator(db, testJWTSecret, provider)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authr := &OIDCAuthenticator{
		db:        db,
		basicAuth: basicAuth,
		rbac:      provider,
	}

	resp, err := authr.loginWithVerifiedClaims("https://issuer.example.com", "", oidcLoginClaims{
		Email:  "alice@example.com",
		Sub:    "alice-subject",
		Groups: []string{"engineering"},
	})
	if err != nil {
		t.Fatalf("OIDC login: %v", err)
	}

	claims, err := basicAuth.validateToken(resp.Token)
	if err != nil {
		t.Fatalf("validate reconciled token: %v", err)
	}
	if claims.AuthorizationSyncedAt == nil {
		t.Fatal("expected OIDC token to carry authorization sync timestamp")
	}
}
