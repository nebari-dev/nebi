package localserver

import (
	"fmt"
	"net"
)

// FindAvailablePort tries ports starting from DefaultBasePort until it finds one available.
// It tries up to MaxPortAttempts ports before giving up.
func FindAvailablePort() (int, error) {
	for i := 0; i < MaxPortAttempts; i++ {
		port := DefaultBasePort + i
		if isPortAvailable(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", DefaultBasePort, DefaultBasePort+MaxPortAttempts-1)
}

// isPortAvailable checks if a TCP port is available for binding.
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
