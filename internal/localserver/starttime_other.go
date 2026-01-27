//go:build !linux

package localserver

// getProcessStartTime returns the start time of a process.
// On non-Linux platforms, this returns 0 which disables the start time check
// and falls back to PID-only stale lock detection.
func getProcessStartTime(_ int) int64 {
	return 0
}
