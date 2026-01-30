package localstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ServerCredential stores auth info for a single nebi server.
type ServerCredential struct {
	Token    string `json:"token"`
	Username string `json:"username,omitempty"`
}

// Credentials maps server URLs to their auth tokens.
type Credentials struct {
	Servers map[string]*ServerCredential `json:"servers"`
}

// CredentialsPath returns the path to credentials.json in the data directory.
func CredentialsPath() (string, error) {
	dir, err := defaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// LoadCredentials reads credentials from disk. Returns empty credentials if not found.
func LoadCredentials() (*Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Credentials{Servers: make(map[string]*ServerCredential)}, nil
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	if creds.Servers == nil {
		creds.Servers = make(map[string]*ServerCredential)
	}
	return &creds, nil
}

// SaveCredentials writes credentials to disk.
func SaveCredentials(creds *Credentials) error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	return SaveCredentialsTo(path, creds)
}

// SaveCredentialsTo writes credentials to a specific path.
func SaveCredentialsTo(path string, creds *Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	return nil
}
