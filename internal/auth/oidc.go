package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// OIDCAuthenticator provides generic OIDC authentication
type OIDCAuthenticator struct {
	provider  *oidc.Provider
	config    *oauth2.Config
	verifier  *oidc.IDTokenVerifier
	db        *gorm.DB
	basicAuth *BasicAuthenticator
	rbac      rbac.Provider
}

// OIDCConfig holds OIDC configuration
type OIDCConfig struct {
	IssuerURL    string
	DiscoveryURL string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

type oidcLoginClaims struct {
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Sub               string   `json:"sub"`
	Picture           string   `json:"picture"`
	Groups            []string `json:"groups"`
}

// NewOIDCAuthenticator creates a new OIDC authenticator
func NewOIDCAuthenticator(ctx context.Context, cfg OIDCConfig, db *gorm.DB, jwtSecret string, rbacProvider rbac.Provider) (*OIDCAuthenticator, error) {
	// Use background context if none provided
	if ctx == nil {
		ctx = context.Background()
	}

	// Discover OIDC provider configuration.
	//
	// In split-horizon deployments (e.g. Keycloak behind an external gateway
	// while pods reach it via an in-cluster Service) the URL nebi uses to fetch
	// .well-known/openid-configuration differs from the issuer that appears in
	// the token's "iss" claim. When DiscoveryURL is set we fetch discovery from
	// it but keep validating "iss" against IssuerURL, using
	// oidc.InsecureIssuerURLContext to tell go-oidc the two are intentionally
	// different. When DiscoveryURL is empty the behavior is unchanged: discovery
	// and issuer validation both use IssuerURL.
	discoveryURL := cfg.DiscoveryURL
	if discoveryURL != "" && discoveryURL != cfg.IssuerURL {
		ctx = oidc.InsecureIssuerURLContext(ctx, cfg.IssuerURL)
	} else {
		discoveryURL = cfg.IssuerURL
	}

	provider, err := oidc.NewProvider(ctx, discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	// Default scopes if none provided
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
	}

	// Configure OAuth2
	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	// Create ID token verifier
	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	// Create basic authenticator for JWT generation
	basicAuth, err := NewBasicAuthenticator(db, jwtSecret, rbacProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create basic authenticator: %w", err)
	}

	return &OIDCAuthenticator{
		provider:  provider,
		config:    oauth2Config,
		verifier:  verifier,
		db:        db,
		basicAuth: basicAuth,
		rbac:      rbacProvider,
	}, nil
}

// Verifier returns the OIDC ID token verifier for signature validation.
func (a *OIDCAuthenticator) Verifier() *oidc.IDTokenVerifier {
	return a.verifier
}

// GetAuthURL returns the URL to redirect users to for authentication
func (a *OIDCAuthenticator) GetAuthURL(state string) string {
	return a.config.AuthCodeURL(state)
}

// HandleCallback handles the OAuth2 callback
func (a *OIDCAuthenticator) HandleCallback(ctx context.Context, code string) (*LoginResponse, error) {
	// Exchange code for token
	oauth2Token, err := a.config.Exchange(ctx, code)
	if err != nil {
		logIdentityProviderAuthFailure(authReconciliationOIDCGroups, err)
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Extract ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		err := errors.New("no id_token in token response")
		logIdentityProviderAuthFailure(authReconciliationOIDCGroups, err)
		return nil, err
	}

	// Verify ID token
	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		logIdentityProviderAuthFailure(authReconciliationOIDCGroups, err)
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	// Extract claims
	var claims oidcLoginClaims
	if err := idToken.Claims(&claims); err != nil {
		logIdentityProviderAuthFailure(authReconciliationOIDCGroups, err)
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Do not log identity claims (email, name, subject, picture) - they are PII
	// and end up in aggregated log stores. A presence marker is enough for ops.
	slog.Debug("OIDC login claims parsed")

	return a.loginWithVerifiedClaims(idToken.Issuer, idToken.Subject, claims)
}

func (a *OIDCAuthenticator) loginWithVerifiedClaims(issuer, tokenSubject string, claims oidcLoginClaims) (*LoginResponse, error) {
	// Find or create user
	subject := tokenSubject
	if subject == "" {
		subject = claims.Sub
	}
	user, err := a.findOrCreateUser(federatedUserClaims{
		Issuer:            issuer,
		Subject:           subject,
		PreferredUsername: claims.PreferredUsername,
		Email:             claims.Email,
		EmailVerified:     claims.EmailVerified,
		Name:              claims.Name,
		AvatarURL:         claims.Picture,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find or create user: %w", err)
	}

	if err := syncOIDCGroups(a.db, user.ID, claims.Groups, a.rbac); err != nil {
		slog.Error("OIDC group sync failed; rejecting login", "user_id", user.ID, "error", err)
		return nil, fmt.Errorf("sync oidc groups: %w", err)
	}

	// Generate JWT token using existing system
	token, err := a.basicAuth.generateReconciledToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	slog.Info("User logged in via OIDC", "user_id", user.ID, "username", user.Username)
	return &LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

// findOrCreateUser finds an existing federated user or creates a new one.
func (a *OIDCAuthenticator) findOrCreateUser(claims federatedUserClaims) (*models.User, error) {
	return findOrCreateFederatedUser(a.db, claims)
}
