package localserver

import (
	"fmt"
	"os"
	"path/filepath"
)

// Lock represents a file-based lock to prevent race conditions when spawning the server.
type Lock struct {
	path string
	file *os.File
}

// AcquireLock acquires the spawn lock file. Returns an error if the lock cannot be acquired.
// The lock is advisory and uses O_CREATE|O_EXCL for atomic creation.
func AcquireLock() (*Lock, error) {
	lockPath, err := GetLockPath()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Try to create the lock file exclusively.
	// If the file already exists, another process is spawning.
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Check if the lock is stale (the owning process may have crashed).
			if isLockStale(lockPath) {
				// Remove stale lock and retry.
				os.Remove(lockPath)
				file, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					return nil, fmt.Errorf("failed to acquire spawn lock after removing stale lock: %w", err)
				}
			} else {
				return nil, fmt.Errorf("another process is already spawning the server")
			}
		} else {
			return nil, fmt.Errorf("failed to acquire spawn lock: %w", err)
		}
	}

	// Write our PID to the lock file for stale lock detection.
	fmt.Fprintf(file, "%d", os.Getpid())

	return &Lock{
		path: lockPath,
		file: file,
	}, nil
}

// Release releases the spawn lock.
func (l *Lock) Release() error {
	if l.file != nil {
		l.file.Close()
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to release spawn lock: %w", err)
	}
	return nil
}

// isLockStale checks if a lock file is stale by reading the PID and checking if it's alive.
func isLockStale(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		// Can't read the lock file; assume it's stale.
		return true
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		// Can't parse PID; assume stale.
		return true
	}

	return !IsProcessAlive(pid)
}
