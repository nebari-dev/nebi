//go:build unix

package jobs

import "golang.org/x/sys/unix"

// processRunning reports whether process pid exists. Signal 0 only checks
// that it could be signalled.
func processRunning(pid int) error {
	return unix.Kill(pid, 0)
}
