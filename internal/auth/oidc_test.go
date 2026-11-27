package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/auth/authtest"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

const testClientID = "nebi"

type oidcFixture struct {
	idp  *authtest.Server
	db   *gorm.DB
	auth *OIDCAuthenticator
}

func newOIDCFixture(t *testing.T, rbacProvider rbac.Provider) *oidcFixture {
	t.Helper()
	idp, err := authtest.NewServer(testClientID)
	if err != nil {
		t.Fatalf("start idp: %v", err)
	}
	t.Cleanup(idp.Close)

	db := setupTestDB(t)
	if rbacProvider == nil {
		if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
			t.Fatalf("rbac: %v", err)
		}
		rbacProvider = rbac.NewDefaultProvider()
	}
	a := NewOIDCAuthenticator(OIDCConfig{
		IssuerURL:   idp.URL,
		ClientID:    testClientID,
		AdminGroups: []string{"nebi-admin"},
	}, db, rbacProvider)
	if err := a.Discover(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	return &oidcFixture{idp: idp, db: db, auth: a}
}

// call runs the middleware and returns the status and the authenticated user.
func (f *oidcFixture) call(t *testing.T, authorization string) (int, *models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var user *models.User
	r := gin.New()
	r.GET("/", f.auth.Middleware(), func(c *gin.Context) {
		u, err := UserFromContext(c)
		if err != nil {
			t.Fatalf("no user in context: %v", err)
		}
		user = u
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["error"] == "" {
			t.Fatalf("expected an error message for status %d, got %q", rec.Code, rec.Body.String())
		}
	}
	return rec.Code, user
}

var alice = authtest.Identity{
	Subject:       "sub-alice",
	Username:      "alice",
	Email:         "alice@example.com",
	EmailVerified: true,
	Groups:        []string{"data-science", "/nebi-admin"},
}

func TestOIDCAuthenticator_ProvisionsUserAndSyncsGroupsAndAdmin(t *testing.T) {
	f := newOIDCFixture(t, nil)

	status, user := f.call(t, "Bearer "+f.idp.Token(alice))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if user.Username != "alice" {
		t.Fatalf("expected alice, got %s", user.Username)
	}
	var identity models.FederatedIdentity
	if err := f.db.First(&identity, "issuer = ? AND subject = ?", f.idp.URL, "sub-alice").Error; err != nil {
		t.Fatalf("expected federated identity bound to issuer and subject: %v", err)
	}
	groups, _ := rbac.GetUserGroups(user.ID)
	if len(groups) != 2 {
		t.Fatalf("expected 2 group memberships, got %v", groups)
	}
	if isAdmin, _ := rbac.IsAdmin(user.ID); !isAdmin {
		t.Fatal("expected admin from the nebi-admin group")
	}

	// A new token without the admin group revokes admin and the membership.
	demoted := alice
	demoted.Groups = []string{"data-science"}
	if status, _ := f.call(t, "Bearer "+f.idp.Token(demoted)); status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if isAdmin, _ := rbac.IsAdmin(user.ID); isAdmin {
		t.Fatal("expected admin to be revoked")
	}
	if groups, _ := rbac.GetUserGroups(user.ID); len(groups) != 1 {
		t.Fatalf("expected 1 group membership, got %v", groups)
	}
}

func TestOIDCAuthenticator_RejectsBadRequests(t *testing.T) {
	f := newOIDCFixture(t, nil)
	other, err := authtest.NewServer(testClientID)
	if err != nil {
		t.Fatalf("start second idp: %v", err)
	}
	t.Cleanup(other.Close)

	tests := []struct {
		name   string
		header string
	}{
		{"missing", ""},
		{"not bearer", "Basic YWxpY2U6cGFzcw=="},
		{"empty bearer", "Bearer "},
		{"garbage", "Bearer not-a-jwt"},
		{"wrong audience", "Bearer " + f.idp.TokenWithClaims(alice, map[string]any{"aud": "someone-else"})},
		{"expired", "Bearer " + f.idp.TokenWithClaims(alice, map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})},
		{"other issuer", "Bearer " + other.Token(alice)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, _ := f.call(t, tt.header); status != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", status)
			}
		})
	}
	var users int64
	f.db.Model(&models.User{}).Count(&users)
	if users != 0 {
		t.Fatalf("expected no user to be provisioned, got %d", users)
	}
}

func TestOIDCAuthenticator_UnavailableBeforeDiscovery(t *testing.T) {
	idp, err := authtest.NewServer(testClientID)
	if err != nil {
		t.Fatalf("start idp: %v", err)
	}
	t.Cleanup(idp.Close)
	f := &oidcFixture{idp: idp, db: setupTestDB(t)}
	f.auth = NewOIDCAuthenticator(OIDCConfig{IssuerURL: idp.URL, ClientID: testClientID}, f.db, &stubRBACProvider{})

	if status, _ := f.call(t, "Bearer "+idp.Token(alice)); status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before discovery, got %d", status)
	}
	if err := f.auth.Discover(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if status, _ := f.call(t, "Bearer "+idp.Token(alice)); status != http.StatusOK {
		t.Fatalf("expected 200 after discovery, got %d", status)
	}
}

func TestOIDCAuthenticator_FailedReconciliationRejectsAndIsNotCached(t *testing.T) {
	provider := &stubRBACProvider{addUserToGroupErr: errors.New("casbin down")}
	f := newOIDCFixture(t, provider)
	token := "Bearer " + f.idp.Token(alice)

	if status, _ := f.call(t, token); status != http.StatusInternalServerError {
		t.Fatalf("expected 500 when group sync fails, got %d", status)
	}
	provider.addUserToGroupErr = nil
	if status, _ := f.call(t, token); status != http.StatusOK {
		t.Fatalf("expected the same token to be reconciled again, got %d", status)
	}
	if !provider.madeAdmin {
		t.Fatal("expected admin to be granted on the successful retry")
	}
}

func TestOIDCAuthenticator_CachesReconciliationPerToken(t *testing.T) {
	provider := &stubRBACProvider{}
	f := newOIDCFixture(t, provider)
	now := time.Now()
	f.auth.now = func() time.Time { return now }
	token := "Bearer " + f.idp.Token(alice)

	if status, _ := f.call(t, token); status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(provider.addedGroups) != 2 {
		t.Fatalf("expected 2 groups added, got %v", provider.addedGroups)
	}

	// Within the cache window the claims are not applied again.
	provider.addUserToGroupErr = errors.New("must not be called")
	if status, _ := f.call(t, token); status != http.StatusOK {
		t.Fatalf("expected cached token to be accepted, got %d", status)
	}

	// After the cache window the token is verified and reconciled again.
	now = now.Add(authorizationCacheTTL + time.Second)
	if status, _ := f.call(t, token); status != http.StatusInternalServerError {
		t.Fatalf("expected reconciliation after the cache window, got %d", status)
	}
}

func TestOIDCAuthenticator_ParallelRequestsWithOneTokenReconcileOnce(t *testing.T) {
	provider := &stubRBACProvider{}
	f := newOIDCFixture(t, provider)
	token := "Bearer " + f.idp.Token(alice)

	var wg sync.WaitGroup
	statuses := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _ := f.call(t, token)
			statuses <- status
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
	}
	if len(provider.addedGroups) != 2 {
		t.Fatalf("expected one reconciliation adding 2 groups, got %v", provider.addedGroups)
	}
	var users int64
	f.db.Model(&models.User{}).Count(&users)
	if users != 1 {
		t.Fatalf("expected one user, got %d", users)
	}
}

// newDiscoveryServer serves an OIDC discovery document advertising issuer
// regardless of the URL used to reach it, like a Keycloak reachable
// in-cluster at one URL while its tokens carry the public issuer.
func newDiscoveryServer(t *testing.T, issuer string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/protocol/openid-connect/auth",
			"token_endpoint":                        srv.URL + "/protocol/openid-connect/token",
			"jwks_uri":                              srv.URL + "/protocol/openid-connect/certs",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestOIDCAuthenticator_DiscoveryURLAllowsIssuerMismatch(t *testing.T) {
	const publicIssuer = "https://keycloak.example.com/realms/nebari"
	srv := newDiscoveryServer(t, publicIssuer)

	a := NewOIDCAuthenticator(OIDCConfig{IssuerURL: publicIssuer, DiscoveryURL: srv.URL, ClientID: testClientID}, nil, nil)
	if err := a.Discover(context.Background()); err != nil {
		t.Fatalf("expected discovery via DiscoveryURL to succeed, got error: %v", err)
	}
}

func TestOIDCAuthenticator_IssuerMismatchWithoutDiscoveryURLFails(t *testing.T) {
	srv := newDiscoveryServer(t, "https://keycloak.example.com/realms/nebari")

	a := NewOIDCAuthenticator(OIDCConfig{IssuerURL: srv.URL, ClientID: testClientID}, nil, nil)
	if err := a.Discover(context.Background()); err == nil {
		t.Fatal("expected issuer mismatch to fail without DiscoveryURL")
	}
}

func TestShouldBeAdminFromGroups(t *testing.T) {
	admins := []string{" nebi-admin ", "/ops"}
	for groups, want := range map[string]bool{
		"nebi-admin":  true,
		"/nebi-admin": true,
		"ops":         true,
		"developers":  false,
		"":            false,
	} {
		if got := shouldBeAdminFromGroups([]string{groups}, admins); got != want {
			t.Errorf("groups %q: got %v, want %v", groups, got, want)
		}
	}
}

func TestSyncAdminRole_ReturnsProviderErrors(t *testing.T) {
	wantErr := errors.New("boom")
	if err := syncAdminRole([16]byte{}, true, &stubRBACProvider{isAdminErr: wantErr}); !errors.Is(err, wantErr) {
		t.Errorf("expected admin check error, got %v", err)
	}
	if err := syncAdminRole([16]byte{}, true, &stubRBACProvider{makeAdminErr: wantErr}); !errors.Is(err, wantErr) {
		t.Errorf("expected grant error, got %v", err)
	}
	if err := syncAdminRole([16]byte{}, false, &stubRBACProvider{isAdmin: true, revokeAdminErr: wantErr}); !errors.Is(err, wantErr) {
		t.Errorf("expected revoke error, got %v", err)
	}
}
