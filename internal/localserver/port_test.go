package localserver

import (
	"net"
	"testing"
)

func TestFindAvailablePort(t *testing.T) {
	port, err := FindAvailablePort()
	if err != nil {
		t.Fatalf("FindAvailablePort failed: %v", err)
	}

	if port < DefaultBasePort || port >= DefaultBasePort+MaxPortAttempts {
		t.Errorf("Port %d is outside expected range [%d, %d)", port, DefaultBasePort, DefaultBasePort+MaxPortAttempts)
	}
}

func TestFindAvailablePort_SkipsOccupied(t *testing.T) {
	// Occupy the default port.
	ln, err := net.Listen("tcp", ":8460")
	if err != nil {
		t.Skipf("Cannot bind to port 8460: %v", err)
	}
	defer ln.Close()

	port, err := FindAvailablePort()
	if err != nil {
		t.Fatalf("FindAvailablePort failed: %v", err)
	}

	// Should find a port > 8460 since 8460 is occupied.
	if port == 8460 {
		t.Error("Should not return occupied port 8460")
	}
	if port < DefaultBasePort || port >= DefaultBasePort+MaxPortAttempts {
		t.Errorf("Port %d is outside expected range", port)
	}
}

func TestIsPortAvailable(t *testing.T) {
	// Find an available port first.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	occupiedPort := ln.Addr().(*net.TCPAddr).Port

	// Port should not be available while occupied.
	if isPortAvailable(occupiedPort) {
		t.Error("Port should not be available while in use")
	}

	ln.Close()

	// Port should now be available.
	if !isPortAvailable(occupiedPort) {
		t.Error("Port should be available after closing listener")
	}
}
