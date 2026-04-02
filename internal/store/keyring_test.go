package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func init() {
	// Use mock keyring backend for tests
	keyring.MockInit()
}

func TestKeyring_SetGetDelete(t *testing.T) {
	kr := &KeyringStore{}

	if err := kr.SetPassword("test-registry", "secret123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	pw, err := kr.GetPassword("test-registry")
	if err != nil {
		t.Fatalf("GetPassword: %v", err)
	}
	if pw != "secret123" {
		t.Fatalf("expected 'secret123', got %q", pw)
	}

	if err := kr.DeletePassword("test-registry"); err != nil {
		t.Fatalf("DeletePassword: %v", err)
	}

	_, err = kr.GetPassword("test-registry")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestKeyring_GetNotFound(t *testing.T) {
	kr := &KeyringStore{}

	_, err := kr.GetPassword("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestFileFallback_SetGetDelete(t *testing.T) {
	dir := t.TempDir()
	fb := &FileCredentialStore{Path: filepath.Join(dir, "credentials.json")}

	if err := fb.SetPassword("test-registry", "secret123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(fb.Path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}

	pw, err := fb.GetPassword("test-registry")
	if err != nil {
		t.Fatalf("GetPassword: %v", err)
	}
	if pw != "secret123" {
		t.Fatalf("expected 'secret123', got %q", pw)
	}

	if err := fb.DeletePassword("test-registry"); err != nil {
		t.Fatalf("DeletePassword: %v", err)
	}

	_, err = fb.GetPassword("test-registry")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestFileFallback_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	fb := &FileCredentialStore{Path: filepath.Join(dir, "credentials.json")}

	_, err := fb.GetPassword("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}
