package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// OIDCAuthenticator provides generic OIDC authentication
type OIDCAuthenticator struct {
	provider    *oidc.Provider
	config      *oauth2.Config
	verifier    *oidc.IDTokenVerifier
	db          *gorm.DB
	jwtSecret   []byte
	basicAuth   *BasicAuthenticator
	adminGroups []string
}

// OIDCConfig holds OIDC configuration
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	AdminGroups  string // Comma-separated groups that grant admin role
}

// NewOIDCAuthenticator creates a new OIDC authenticator
func NewOIDCAuthenticator(ctx context.Context, cfg OIDCConfig, db *gorm.DB, jwtSecret string) (*OIDCAuthenticator, error) {
	// Use background context if none provided
	if ctx == nil {
		ctx = context.Background()
	}

	// Discover OIDC provider configuration
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
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
	basicAuth := NewBasicAuthenticator(db, jwtSecret)

	return &OIDCAuthenticator{
		provider:    provider,
		config:      oauth2Config,
		verifier:    verifier,
		db:          db,
		jwtSecret:   []byte(jwtSecret),
		basicAuth:   basicAuth,
		adminGroups: parseAdminGroups(cfg.AdminGroups),
	}, nil
}

// GetAuthURL returns the URL to redirect users to for authentication
func (a *OIDCAuthenticator) GetAuthURL(state string) string {
	return a.config.AuthCodeURL(state)
}

// GetAuthURLWithRedirect returns the auth URL using a custom redirect URI.
// Used by CLI login which has its own callback endpoint.
func (a *OIDCAuthenticator) GetAuthURLWithRedirect(state, redirectURL string) string {
	return a.config.AuthCodeURL(state, oauth2.SetAuthURLParam("redirect_uri", redirectURL))
}

// HandleCallback handles the OAuth2 callback
func (a *OIDCAuthenticator) HandleCallback(ctx context.Context, code string) (*LoginResponse, error) {
	oauth2Token, err := a.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	return a.processIDToken(ctx, oauth2Token)
}

// HandleCallbackWithRedirect handles the OAuth2 callback using a custom redirect URI.
// The redirect_uri must match what was used in GetAuthURLWithRedirect.
func (a *OIDCAuthenticator) HandleCallbackWithRedirect(ctx context.Context, code, redirectURL string) (*LoginResponse, error) {
	oauth2Token, err := a.config.Exchange(ctx, code, oauth2.SetAuthURLParam("redirect_uri", redirectURL))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	return a.processIDToken(ctx, oauth2Token)
}

// processIDToken extracts user info from the OAuth2 token response and returns a Nebi JWT.
func (a *OIDCAuthenticator) processIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*LoginResponse, error) {
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("no id_token in token response")
	}

	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims struct {
		Email             string   `json:"email"`
		EmailVerified     bool     `json:"email_verified"`
		Name              string   `json:"name"`
		PreferredUsername string   `json:"preferred_username"`
		Sub               string   `json:"sub"`
		Picture           string   `json:"picture"`
		Groups            []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	slog.Info("OIDC login claims", "email", claims.Email, "name", claims.Name, "sub", claims.Sub, "picture", claims.Picture, "groups", claims.Groups)

	username := claims.Email
	if username == "" {
		username = claims.PreferredUsername
	}
	if username == "" {
		username = claims.Sub
	}

	user, err := a.findOrCreateUser(username, claims.Email, claims.Name, claims.Picture, claims.Sub)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create user: %w", err)
	}

	// Sync admin role based on OIDC group membership
	if len(a.adminGroups) > 0 {
		syncRolesFromGroups(user.ID, claims.Groups, a.adminGroups)
	}

	token, err := a.basicAuth.generateToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	slog.Info("User logged in via OIDC", "user_id", user.ID, "username", user.Username)
	return &LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

// findOrCreateUser finds an existing user or creates a new one
func (a *OIDCAuthenticator) findOrCreateUser(username, email, name, avatarURL, oidcSub string) (*models.User, error) {
	var user models.User

	// Try to find user by username or email
	result := a.db.Where("username = ? OR email = ?", username, email).First(&user)
	if result.Error == nil {
		// User exists - update avatar if it changed
		if user.AvatarURL != avatarURL {
			user.AvatarURL = avatarURL
			a.db.Save(&user)
		}
		return &user, nil
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error: %w", result.Error)
	}

	// Create new user
	user = models.User{
		ID:        uuid.New(),
		Username:  username,
		Email:     email,
		AvatarURL: avatarURL,
		// No password hash - OIDC users don't have passwords
		PasswordHash: "",
	}

	if err := a.db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	slog.Info("Created new user from OIDC", "user_id", user.ID, "username", user.Username, "email", email)
	return &user, nil
}
