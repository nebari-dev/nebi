package localserver

import (
	"fmt"
	"os"
	"syscall"
)

// IsProcessAlive checks if a process with the given PID is still running.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if the process exists without killing it.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// StopProcess sends SIGTERM to the process with the given PID.
func StopProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to process %d: %w", pid, err)
	}

	return nil
}
