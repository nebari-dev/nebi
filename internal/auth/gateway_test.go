package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseGatewayToken_HappyPath(t *testing.T) {
	claims := ProxyTokenClaims{
		Sub:              "sub-123",
		PreferredUsername: "alice",
		Email:            "alice@example.com",
		Name:             "Alice",
		Groups:           []string{"admin", "dev"},
	}
	jwt := makeJWT(t, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)

	got, err := ParseGatewayToken(req)
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

func TestParseGatewayToken_NoHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ParseGatewayToken(req)
	if err == nil {
		t.Error("expected error when no Authorization header")
	}
}

func TestParseGatewayToken_InvalidFormat(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	_, err := ParseGatewayToken(req)
	if err == nil {
		t.Error("expected error for non-Bearer auth")
	}
}

func TestParseGatewayToken_InvalidJWT(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	_, err := ParseGatewayToken(req)
	if err == nil {
		t.Error("expected error for invalid JWT")
	}
}
