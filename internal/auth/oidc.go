package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openteams-ai/darb/internal/models"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// OIDCAuthenticator provides generic OIDC authentication
type OIDCAuthenticator struct {
	provider  *oidc.Provider
	config    *oauth2.Config
	verifier  *oidc.IDTokenVerifier
	db        *gorm.DB
	jwtSecret []byte
	basicAuth *BasicAuthenticator
}

// OIDCConfig holds OIDC configuration
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
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
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
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
		provider:  provider,
		config:    oauth2Config,
		verifier:  verifier,
		db:        db,
		jwtSecret: []byte(jwtSecret),
		basicAuth: basicAuth,
	}, nil
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
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Extract ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("no id_token in token response")
	}

	// Verify ID token
	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	// Extract claims
	var claims struct {
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Sub               string `json:"sub"`
		Picture           string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	slog.Info("OIDC login claims", "email", claims.Email, "name", claims.Name, "sub", claims.Sub, "picture", claims.Picture)

	// Determine username
	username := claims.Email
	if username == "" {
		username = claims.PreferredUsername
	}
	if username == "" {
		username = claims.Sub
	}

	// Find or create user
	user, err := a.findOrCreateUser(username, claims.Email, claims.Name, claims.Picture, claims.Sub)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create user: %w", err)
	}

	// Generate JWT token using existing system
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
