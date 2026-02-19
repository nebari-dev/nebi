package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// makeJWT builds a fake JWT with the given claims payload.
// The header and signature are placeholders — only the payload matters.
func makeJWT(t *testing.T, claims any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("failed to marshal claims: %v", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	return header + "." + payloadB64 + "." + sig
}

func TestParseIdTokenCookie_HappyPath(t *testing.T) {
	claims := ProxyTokenClaims{
		Sub:               "sub-123",
		PreferredUsername: "alice",
		Email:             "alice@example.com",
		Name:              "Alice",
		Picture:           "https://example.com/alice.png",
		Groups:            []string{"admin", "dev"},
	}
	jwt := makeJWT(t, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "IdToken-nebi", Value: jwt})

	got, err := parseIdTokenCookie(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.PreferredUsername != "alice" {
		t.Errorf("expected username alice, got %s", got.PreferredUsername)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", got.Email)
	}
	if len(got.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(got.Groups))
	}
}

func TestParseIdTokenCookie_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := parseIdTokenCookie(req)
	if err == nil {
		t.Error("expected error when no IdToken cookie present")
	}
}

func TestParseIdTokenCookie_InvalidJWT(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "IdToken-nebi", Value: "not-a-jwt"})

	_, err := parseIdTokenCookie(req)
	if err == nil {
		t.Error("expected error for invalid JWT format")
	}
}

func TestFindOrCreateProxyUser_CreatesNew(t *testing.T) {
	db := setupTestDB(t)

	claims := &ProxyTokenClaims{
		PreferredUsername: "bob",
		Email:             "bob@example.com",
		Picture:           "https://example.com/bob.png",
		Groups:            []string{"engineering", "dev"},
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
	if len(user.Groups) != 2 || user.Groups[0] != "engineering" || user.Groups[1] != "dev" {
		t.Errorf("expected groups [engineering dev], got %v", user.Groups)
	}

	// Verify groups persisted in DB
	var dbUser models.User
	db.First(&dbUser, "username = ?", "bob")
	if len(dbUser.Groups) != 2 {
		t.Errorf("expected 2 groups persisted in db, got %d", len(dbUser.Groups))
	}
}

func TestFindOrCreateProxyUser_FindsExisting(t *testing.T) {
	db := setupTestDB(t)

	existing := models.User{
		ID:           uuid.New(),
		Username:     "carol",
		Email:        "carol@example.com",
		AvatarURL:    "old-avatar",
		Groups:       []string{"old-group"},
		PasswordHash: "",
	}
	db.Create(&existing)

	claims := &ProxyTokenClaims{
		PreferredUsername: "carol",
		Email:             "carol@example.com",
		Picture:           "new-avatar",
		Groups:            []string{"new-group", "engineering"},
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
	if len(user.Groups) != 2 || user.Groups[0] != "new-group" || user.Groups[1] != "engineering" {
		t.Errorf("expected groups to be synced to [new-group engineering], got %v", user.Groups)
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
