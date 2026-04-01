package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/zalando/go-keyring"
)

const keyringService = "nebi"

// CredentialStore provides password storage for registry credentials.
type CredentialStore interface {
	SetPassword(registryName, password string) error
	GetPassword(registryName string) (string, error)
	DeletePassword(registryName string) error
}

// KeyringStore stores credentials in the OS keychain via go-keyring.
type KeyringStore struct{}

func keyringKey(registryName string) string {
	return "registry/" + registryName + "/password"
}

func (k *KeyringStore) SetPassword(registryName, password string) error {
	return keyring.Set(keyringService, keyringKey(registryName), password)
}

func (k *KeyringStore) GetPassword(registryName string) (string, error) {
	pw, err := keyring.Get(keyringService, keyringKey(registryName))
	if err != nil {
		return "", fmt.Errorf("credential not found for registry %q", registryName)
	}
	return pw, nil
}

func (k *KeyringStore) DeletePassword(registryName string) error {
	return keyring.Delete(keyringService, keyringKey(registryName))
}

// FileCredentialStore is a plaintext fallback for environments without a keychain.
type FileCredentialStore struct {
	Path string
	mu   sync.Mutex
}

func (f *FileCredentialStore) load() (map[string]string, error) {
	data, err := os.ReadFile(f.Path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	var creds map[string]string
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials file: %w", err)
	}
	return creds, nil
}

func (f *FileCredentialStore) save(creds map[string]string) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, data, 0600)
}

func (f *FileCredentialStore) SetPassword(registryName, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	creds, err := f.load()
	if err != nil {
		return err
	}
	creds[keyringKey(registryName)] = password
	return f.save(creds)
}

func (f *FileCredentialStore) GetPassword(registryName string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	creds, err := f.load()
	if err != nil {
		return "", err
	}
	pw, ok := creds[keyringKey(registryName)]
	if !ok {
		return "", fmt.Errorf("credential not found for registry %q", registryName)
	}
	return pw, nil
}

func (f *FileCredentialStore) DeletePassword(registryName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	creds, err := f.load()
	if err != nil {
		return err
	}
	delete(creds, keyringKey(registryName))
	return f.save(creds)
}

// NewCredentialStore returns a KeyringStore if available, otherwise a FileCredentialStore.
// Prints a warning to stderr when falling back to file-based storage.
func NewCredentialStore(dataDir string) CredentialStore {
	// Test if the keyring is available by doing a set+delete
	testKey := "nebi-keyring-test"
	if err := keyring.Set(keyringService, testKey, "test"); err == nil {
		keyring.Delete(keyringService, testKey)
		return &KeyringStore{}
	}

	path := dataDir + "/credentials.json"
	fmt.Fprintf(os.Stderr, "Warning: OS keychain unavailable, credentials stored in plaintext at %s\n", path)
	return &FileCredentialStore{Path: path}
}
