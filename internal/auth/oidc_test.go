package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	if review.CollisionField != "username" {
		t.Errorf("expected username collision field, got %s", review.CollisionField)
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
