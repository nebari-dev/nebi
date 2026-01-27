package localserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndReadState(t *testing.T) {
	// Use a temp directory for the test.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state := &ServerState{
		PID:       12345,
		Port:      8460,
		Token:     "test-token-abc123",
		StartedAt: time.Now().Truncate(time.Second),
	}

	// Write state.
	if err := WriteState(state); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	// Verify the file exists.
	statePath, _ := GetStatePath()
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("State file was not created")
	}

	// Read state back.
	readState, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if readState == nil {
		t.Fatal("ReadState returned nil")
	}

	if readState.PID != state.PID {
		t.Errorf("PID: got %d, want %d", readState.PID, state.PID)
	}
	if readState.Port != state.Port {
		t.Errorf("Port: got %d, want %d", readState.Port, state.Port)
	}
	if readState.Token != state.Token {
		t.Errorf("Token: got %q, want %q", readState.Token, state.Token)
	}
}

func TestReadState_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState should not error for missing file: %v", err)
	}
	if state != nil {
		t.Fatal("ReadState should return nil for missing file")
	}
}

func TestRemoveState(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write a state file first.
	state := &ServerState{PID: 1, Port: 8460, Token: "tok"}
	if err := WriteState(state); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	// Remove it.
	if err := RemoveState(); err != nil {
		t.Fatalf("RemoveState failed: %v", err)
	}

	// Verify it's gone.
	statePath, _ := GetStatePath()
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("State file should have been removed")
	}
}

func TestRemoveState_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Removing a non-existent file should not error.
	if err := RemoveState(); err != nil {
		t.Fatalf("RemoveState should not error for missing file: %v", err)
	}
}

func TestGetStatePath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := GetStatePath()
	if err != nil {
		t.Fatalf("GetStatePath failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".local", "share", "nebi", "server.state")
	if path != expected {
		t.Errorf("GetStatePath: got %q, want %q", path, expected)
	}
}

func TestGetLockPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := GetLockPath()
	if err != nil {
		t.Fatalf("GetLockPath failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".local", "share", "nebi", "spawn.lock")
	if path != expected {
		t.Errorf("GetLockPath: got %q, want %q", path, expected)
	}
}

func TestGetDBPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := GetDBPath()
	if err != nil {
		t.Fatalf("GetDBPath failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".local", "share", "nebi", "nebi.db")
	if path != expected {
		t.Errorf("GetDBPath: got %q, want %q", path, expected)
	}
}
