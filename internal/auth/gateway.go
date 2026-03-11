package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ParseGatewayToken extracts user claims from the Authorization: Bearer header
// forwarded by Envoy Gateway (ForwardAccessToken). The token is a Keycloak JWT
// whose payload is base64-decoded without signature verification — Envoy already
// validated it before forwarding.
func ParseGatewayToken(r *http.Request) (*ProxyTokenClaims, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("no Authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, errors.New("Authorization header is not Bearer")
	}

	token := parts[1]
	jwtParts := strings.Split(token, ".")
	if len(jwtParts) != 3 {
		return nil, fmt.Errorf("token is not a valid JWT (got %d parts)", len(jwtParts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(jwtParts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode JWT payload: %w", err)
	}

	var claims ProxyTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT claims: %w", err)
	}

	return &claims, nil
}
