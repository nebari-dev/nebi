package localserver

import (
	"fmt"
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

func TestAcquireLock_RecycledPidDetected(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a lock file with our own PID but a fake start time.
	// This simulates a recycled PID: the PID is alive but the start time doesn't match.
	lockPath, _ := GetLockPath()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	pid := os.Getpid()
	// Write a bogus start time that won't match our actual start time.
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d 1", pid)), 0600); err != nil {
		t.Fatalf("Failed to write lock: %v", err)
	}

	// On Linux, the start time won't match, so it should detect PID recycling.
	// On other platforms, getProcessStartTime returns 0 so this test just
	// verifies it doesn't crash.
	startTime := getProcessStartTime(pid)
	if startTime > 0 {
		// Linux: should detect the stale lock.
		lock, err := AcquireLock()
		if err != nil {
			t.Fatalf("AcquireLock should succeed with recycled PID: %v", err)
		}
		lock.Release()
	}
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

func TestIsLockStale_PidOnlyFormat(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := tmpDir + "/test.lock"

	// Old format with just PID (no start time) — dead PID.
	os.WriteFile(lockPath, []byte("999999999"), 0600)
	if !isLockStale(lockPath) {
		t.Error("Dead PID-only lock should be stale")
	}

	// Old format with just PID — our own alive PID (should not be stale).
	os.WriteFile(lockPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0600)
	if isLockStale(lockPath) {
		t.Error("Alive PID-only lock should not be stale")
	}
}
