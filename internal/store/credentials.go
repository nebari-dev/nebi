package store

import "fmt"

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

// SaveServerURL stores the server URL.
func (s *Store) SaveServerURL(url string) error {
	return s.db.Save(&Config{ID: 1, ServerURL: url}).Error
}
