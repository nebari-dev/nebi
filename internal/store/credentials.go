package store

import (
	"context"
	"fmt"

	"github.com/nebari-dev/nebi/internal/oidcclient"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// LoadCredentials reads stored credentials.
func (s *Store) LoadCredentials() (*Credentials, error) {
	var creds Credentials
	if err := s.db.First(&creds, 1).Error; err != nil {
		return &Credentials{}, nil
	}
	return &creds, nil
}

// SaveCredentials writes credentials.
func (s *Store) SaveCredentials(creds *Credentials) error {
	creds.ID = 1
	if err := s.db.Save(creds).Error; err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}
	return nil
}

// LoadServerURL returns the configured server URL.
func (s *Store) LoadServerURL() (string, error) {
	var cfg Config
	if err := s.db.First(&cfg, 1).Error; err != nil {
		return "", nil
	}
	return cfg.ServerURL, nil
}

// ClearCredentials removes stored credentials and server URL.
func (s *Store) ClearCredentials() error {
	if err := s.db.Where("1 = 1").Delete(&Credentials{}).Error; err != nil {
		return fmt.Errorf("clearing credentials: %w", err)
	}
	if err := s.db.Where("1 = 1").Delete(&Config{}).Error; err != nil {
		return fmt.Errorf("clearing config: %w", err)
	}
	return nil
}

// SaveServerURL stores the server URL.
func (s *Store) SaveServerURL(url string) error {
	return s.db.Save(&Config{ID: 1, ServerURL: url}).Error
}

// LoggedIn reports whether a login is stored. A login to a server with
// authentication disabled has a username but no token.
func (c *Credentials) LoggedIn() bool {
	return c.Token != "" || c.Username != ""
}

// OAuthToken returns the stored login as an OAuth2 token.
func (c *Credentials) OAuthToken() *oauth2.Token {
	tok := &oauth2.Token{AccessToken: c.Token, TokenType: "Bearer", RefreshToken: c.RefreshToken}
	if c.TokenExpiry != nil {
		tok.Expiry = *c.TokenExpiry
	}
	return tok
}

// TokenSource serves the stored access token and refreshes it with the
// identity provider when it is about to expire, handing every refreshed
// token to save so the rotated refresh token is kept.
func (c *Credentials) TokenSource(ctx context.Context, save func(*oauth2.Token) error) oauth2.TokenSource {
	return oidcclient.TokenSource(ctx, c.TokenURL, c.ClientID, c.OAuthToken(), save)
}

// SetOAuthToken copies tok into the credentials.
func (c *Credentials) SetOAuthToken(tok *oauth2.Token) {
	c.Token = tok.AccessToken
	if tok.RefreshToken != "" {
		c.RefreshToken = tok.RefreshToken
	}
	c.TokenExpiry = nil
	if !tok.Expiry.IsZero() {
		expiry := tok.Expiry
		c.TokenExpiry = &expiry
	}
}

// SaveToken persists a refreshed token into the singleton credentials row of
// db (the CLI store or the local-mode server database).
func SaveToken(db *gorm.DB, tok *oauth2.Token) error {
	var creds Credentials
	if err := db.First(&creds, 1).Error; err != nil {
		return fmt.Errorf("loading credentials: %w", err)
	}
	creds.SetOAuthToken(tok)
	if err := db.Save(&creds).Error; err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}
	return nil
}
