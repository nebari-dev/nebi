package local

// Owner: U5 (remote). The server token lives in store_credentials (one
// server at a time, see store.Config / store.Credentials). The CLI's
// `nebi login`, nebi-web and the desktop app share these rows, so a
// connection made in one is seen by the others.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
)

// Server auth modes stored in store_credentials.auth_mode.
const (
	AuthModeNone  = "none"
	AuthModeToken = "token"
)

// ServerCredentials is the stored connection to the team server.
type ServerCredentials struct {
	URL      string
	Name     string
	Token    string
	Username string
	AuthMode string // "none" | "token"
	// LastSyncedAt is when the server project list was last fetched.
	LastSyncedAt *time.Time
}

// LoadServerCredentials returns the stored connection; URL is "" when not
// connected.
func (s *Service) LoadServerCredentials(ctx context.Context) (*ServerCredentials, error) {
	db := s.deps.DB.WithContext(ctx)
	var cfg store.Config
	if err := db.First(&cfg, 1).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("reading server config: %w", err)
	}
	var creds store.Credentials
	if err := db.First(&creds, 1).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("reading server credentials: %w", err)
	}
	out := &ServerCredentials{
		URL:          cfg.ServerURL,
		Name:         cfg.ServerName,
		Token:        creds.Token,
		Username:     creds.Username,
		AuthMode:     creds.AuthMode,
		LastSyncedAt: cfg.LastSyncedAt,
	}
	if out.AuthMode == "" {
		// Rows written before #649 (legacy `nebi login`) have no mode.
		out.AuthMode = AuthModeNone
		if out.Token != "" {
			out.AuthMode = AuthModeToken
		}
	}
	if out.URL == "" {
		return &ServerCredentials{AuthMode: AuthModeNone}, nil
	}
	return out, nil
}

// SaveServerCredentials stores (replaces) the connection. The last-synced
// time is kept when the URL does not change.
func (s *Service) SaveServerCredentials(ctx context.Context, c ServerCredentials) error {
	return s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cfg store.Config
		if err := tx.First(&cfg, 1).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		synced := c.LastSyncedAt
		if synced == nil && cfg.ServerURL == c.URL {
			synced = cfg.LastSyncedAt
		}
		if err := tx.Save(&store.Config{ID: 1, ServerURL: c.URL, ServerName: c.Name, LastSyncedAt: synced}).Error; err != nil {
			return fmt.Errorf("saving server config: %w", err)
		}
		mode := c.AuthMode
		if mode == "" {
			mode = AuthModeNone
			if c.Token != "" {
				mode = AuthModeToken
			}
		}
		if err := tx.Save(&store.Credentials{ID: 1, Token: c.Token, Username: c.Username, AuthMode: mode}).Error; err != nil {
			return fmt.Errorf("saving server credentials: %w", err)
		}
		return nil
	})
}

// ClearServerCredentials removes the connection. Project remotes stay.
func (s *Service) ClearServerCredentials(ctx context.Context) error {
	return s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&store.Config{ID: 1}).Error; err != nil {
			return fmt.Errorf("clearing server config: %w", err)
		}
		if err := tx.Save(&store.Credentials{ID: 1}).Error; err != nil {
			return fmt.Errorf("clearing server credentials: %w", err)
		}
		return nil
	})
}

// MarkServerSynced records when the server project list was last fetched
// ("Last synced" on the Server page).
func (s *Service) MarkServerSynced(ctx context.Context, at time.Time) error {
	return s.deps.DB.WithContext(ctx).Model(&store.Config{}).
		Where("id = ? AND server_url <> ''", 1).
		Update("last_synced_at", at.UTC()).Error
}
