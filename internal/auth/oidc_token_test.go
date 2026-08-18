package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
)

const testOIDCClientID = "nebi-test-client"

type oidcTokenFixture struct {
	issuer   string
	rawToken string
	verifier *oidc.IDTokenVerifier
}

func newOIDCTokenFixture(t *testing.T, subject string, extraClaims map[string]any) oidcTokenFixture {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}

	oidcServer := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{{
			PublicKey: priv.Public(),
			KeyID:     "test-key",
			Algorithm: oidc.RS256,
		}},
	}

	var rawToken string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"id_token":     rawToken,
		})
	})
	mux.Handle("/", oidcServer)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oidcServer.SetIssuer(srv.URL)

	claims := map[string]any{
		"iss": srv.URL,
		"aud": testOIDCClientID,
		"sub": subject,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range extraClaims {
		claims[k] = v
	}

	rawClaims, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal token claims: %v", err)
	}
	rawToken = oidctest.SignIDToken(priv, "test-key", oidc.RS256, string(rawClaims))

	provider, err := oidc.NewProvider(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("create OIDC provider: %v", err)
	}

	return oidcTokenFixture{
		issuer:   srv.URL,
		rawToken: rawToken,
		verifier: provider.Verifier(&oidc.Config{ClientID: testOIDCClientID}),
	}
}

func TestVerifyIdTokenCookie_PopulatesIssuerAndSubjectFromVerifiedToken(t *testing.T) {
	fixture := newOIDCTokenFixture(t, "proxy-subject", map[string]any{
		"preferred_username": "proxy-user",
		"email":              "proxy@example.com",
		"email_verified":     true,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "IdToken-nebi", Value: fixture.rawToken})

	claims, err := verifyIdTokenCookie(req, fixture.verifier)
	if err != nil {
		t.Fatalf("verify IdToken cookie: %v", err)
	}

	if claims.Issuer != fixture.issuer {
		t.Fatalf("expected issuer %q, got %q", fixture.issuer, claims.Issuer)
	}
	if claims.Sub != "proxy-subject" {
		t.Fatalf("expected subject proxy-subject, got %q", claims.Sub)
	}
}

func TestExchangeIDToken_PersistsIssuerSubjectFromVerifiedToken(t *testing.T) {
	db := setupTestDB(t)
	localUser := models.User{
		ID:           uuid.New(),
		Username:     "device-user",
		Email:        "device@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}

	fixture := newOIDCTokenFixture(t, "device-subject", map[string]any{
		"preferred_username": "device-remote-user",
		"email":              "device-remote@example.com",
		"email_verified":     true,
	})
	authenticator, err := NewBasicAuthenticator(db, testJWTSecret, rbac.NewDefaultProvider())
	if err != nil {
		t.Fatalf("NewBasicAuthenticator: %v", err)
	}
	authenticator.SetIDTokenVerifier(fixture.verifier)

	resp, err := authenticator.ExchangeIDToken(fixture.rawToken, "")
	if err != nil {
		t.Fatalf("ExchangeIDToken: %v", err)
	}
	if resp.User.ID == localUser.ID {
		t.Fatal("expected device token not to bind by existing username/email")
	}

	var identity models.FederatedIdentity
	if err := db.Where("issuer = ? AND subject = ?", fixture.issuer, "device-subject").First(&identity).Error; err != nil {
		t.Fatalf("load federated identity: %v", err)
	}
	if identity.UserID != resp.User.ID {
		t.Fatalf("expected identity user %s, got %s", resp.User.ID, identity.UserID)
	}
}

func TestOIDCHandleCallback_PersistsIssuerSubjectFromVerifiedToken(t *testing.T) {
	db := setupTestDB(t)
	localUser := models.User{
		ID:           uuid.New(),
		Username:     "callback-user",
		Email:        "callback@example.com",
		PasswordHash: "hashed-password",
	}
	if err := db.Create(&localUser).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}

	fixture := newOIDCTokenFixture(t, "callback-subject", map[string]any{
		"preferred_username": "callback-remote-user",
		"email":              "callback-remote@example.com",
		"email_verified":     true,
	})
	authenticator, err := NewOIDCAuthenticator(context.Background(), OIDCConfig{
		IssuerURL:   fixture.issuer,
		ClientID:    testOIDCClientID,
		RedirectURL: "http://nebi.test/callback",
	}, db, testJWTSecret, rbac.NewDefaultProvider())
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}

	resp, err := authenticator.HandleCallback(context.Background(), "test-code")
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}
	if resp.User.ID == localUser.ID {
		t.Fatal("expected OIDC callback not to bind by existing username/email")
	}

	var identity models.FederatedIdentity
	if err := db.Where("issuer = ? AND subject = ?", fixture.issuer, "callback-subject").First(&identity).Error; err != nil {
		t.Fatalf("load federated identity: %v", err)
	}
	if identity.UserID != resp.User.ID {
		t.Fatalf("expected identity user %s, got %s", resp.User.ID, identity.UserID)
	}
}
