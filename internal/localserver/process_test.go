package localserver

import (
	"os"
	"testing"
)

func TestIsProcessAlive_CurrentProcess(t *testing.T) {
	pid := os.Getpid()
	if !IsProcessAlive(pid) {
		t.Error("Current process should be reported as alive")
	}
}

func TestIsProcessAlive_InvalidPID(t *testing.T) {
	if IsProcessAlive(0) {
		t.Error("PID 0 should not be alive")
	}
	if IsProcessAlive(-1) {
		t.Error("PID -1 should not be alive")
	}
}

func TestIsProcessAlive_NonExistentPID(t *testing.T) {
	// Use a very high PID that's unlikely to exist.
	if IsProcessAlive(999999999) {
		t.Error("PID 999999999 should not be alive")
	}
}
