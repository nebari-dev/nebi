package localserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// Write PID and start time for stale lock detection.
	// Format: "PID START_TIME" — the start time guards against PID recycling.
	startTime := getProcessStartTime(os.Getpid())
	fmt.Fprintf(file, "%d %d", os.Getpid(), startTime)

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

// isLockStale checks if a lock file is stale by verifying the PID is alive
// and its start time matches what was recorded.
func isLockStale(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		// Can't read the lock file; assume it's stale.
		return true
	}

	parts := strings.Fields(string(data))
	if len(parts) == 0 {
		return true
	}

	pid, err := strconv.Atoi(parts[0])
	if err != nil {
		return true
	}

	if !IsProcessAlive(pid) {
		return true
	}

	// PID is alive — check if it's the same process by comparing start times.
	if len(parts) >= 2 {
		recordedStartTime, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil && recordedStartTime > 0 {
			currentStartTime := getProcessStartTime(pid)
			if currentStartTime > 0 && currentStartTime != recordedStartTime {
				// PID was recycled — a different process now has this PID.
				return true
			}
		}
	}

	return false
}
