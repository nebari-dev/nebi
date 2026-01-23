package localserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireAndReleaseLock(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	// Lock file should exist.
	lockPath, _ := GetLockPath()
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("Lock file should exist after acquiring")
	}

	// Release the lock.
	if err := lock.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Lock file should be removed.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("Lock file should be removed after releasing")
	}
}

func TestAcquireLock_DoubleAcquireFails(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	lock1, err := AcquireLock()
	if err != nil {
		t.Fatalf("First AcquireLock failed: %v", err)
	}
	defer lock1.Release()

	// Second acquire should fail since our process is alive.
	_, err = AcquireLock()
	if err == nil {
		t.Fatal("Second AcquireLock should fail while first is held")
	}
}

func TestAcquireLock_StaleLockRemoved(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a stale lock file with a non-existent PID.
	lockPath, _ := GetLockPath()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("999999999"), 0600); err != nil {
		t.Fatalf("Failed to write stale lock: %v", err)
	}

	// Should be able to acquire despite existing lock.
	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock should succeed with stale lock: %v", err)
	}
	lock.Release()
}

func TestIsLockStale_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := tmpDir + "/test.lock"

	// Write invalid content.
	os.WriteFile(lockPath, []byte("not-a-pid"), 0600)
	if !isLockStale(lockPath) {
		t.Error("Lock with invalid PID content should be stale")
	}
}

func TestIsLockStale_NoFile(t *testing.T) {
	if !isLockStale("/nonexistent/path") {
		t.Error("Non-existent lock file should be stale")
	}
}
