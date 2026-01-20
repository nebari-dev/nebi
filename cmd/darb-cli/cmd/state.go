package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// ServerState represents the runtime state of a local server
type ServerState struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"started_at"`
}

// LocalServerPaths holds all relevant paths for local server
type LocalServerPaths struct {
	DataDir   string // ~/.local/share/nebi/
	StateFile string // ~/.local/share/nebi/server.state
	Database  string // ~/.local/share/nebi/nebi.db
	LockFile  string // ~/.local/share/nebi/spawn.lock
	LogDir    string // ~/.local/share/nebi/logs/
	LogFile   string // ~/.local/share/nebi/logs/server.log
}

// getLocalServerPaths returns all paths for local server management
func getLocalServerPaths() (*LocalServerPaths, error) {
	dataDir, err := getDataDir()
	if err != nil {
		return nil, err
	}

	return &LocalServerPaths{
		DataDir:   dataDir,
		StateFile: filepath.Join(dataDir, "server.state"),
		Database:  filepath.Join(dataDir, "nebi.db"),
		LockFile:  filepath.Join(dataDir, "spawn.lock"),
		LogDir:    filepath.Join(dataDir, "logs"),
		LogFile:   filepath.Join(dataDir, "logs", "server.log"),
	}, nil
}

// readServerState reads and parses the server state file
func readServerState() (*ServerState, error) {
	paths, err := getLocalServerPaths()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(paths.StateFile)
	if err != nil {
		return nil, err
	}

	var state ServerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse server state: %w", err)
	}

	return &state, nil
}

// isProcessRunning checks if a process with the given PID is running
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Sending signal 0 checks if process exists without actually sending a signal
	err := syscall.Kill(pid, 0)
	return err == nil
}

// acquireSpawnLock attempts to acquire an exclusive lock for spawning
func acquireSpawnLock() (*os.File, error) {
	paths, err := getLocalServerPaths()
	if err != nil {
		return nil, err
	}

	// Ensure directory exists
	if err := os.MkdirAll(paths.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	lock, err := os.OpenFile(paths.LockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire exclusive lock with timeout
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	lock.Close()
	return nil, fmt.Errorf("timeout acquiring spawn lock (another process may be starting the server)")
}

// releaseSpawnLock releases the spawn lock
func releaseSpawnLock(lock *os.File) {
	if lock != nil {
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		lock.Close()
	}
}

// findAvailablePort finds an available port starting from startPort
func findAvailablePort(startPort int) (int, error) {
	for port := startPort; port < startPort+40; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", startPort, startPort+39)
}

// findServerBinary locates the nebi-server binary
func findServerBinary() (string, error) {
	// First, try next to the CLI binary
	execPath, err := os.Executable()
	if err == nil {
		serverPath := filepath.Join(filepath.Dir(execPath), "nebi-server")
		if _, err := os.Stat(serverPath); err == nil {
			return serverPath, nil
		}
		// Also try darb-server for backwards compatibility
		serverPath = filepath.Join(filepath.Dir(execPath), "darb-server")
		if _, err := os.Stat(serverPath); err == nil {
			return serverPath, nil
		}
	}

	// Try PATH
	if path, err := exec.LookPath("nebi-server"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("darb-server"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("nebi-server binary not found. Ensure it's in your PATH or next to the nebi CLI")
}

// spawnLocalServer spawns a new local server process
func spawnLocalServer(port int) error {
	paths, err := getLocalServerPaths()
	if err != nil {
		return err
	}

	// Ensure log directory exists
	if err := os.MkdirAll(paths.LogDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log file
	logFile, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Find server binary
	serverBinary, err := findServerBinary()
	if err != nil {
		logFile.Close()
		return err
	}

	cmd := exec.Command(serverBinary,
		"-port", fmt.Sprintf("%d", port),
		"-mode", "both",
		"-local",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("DARB_DATABASE_DSN=%s", paths.Database),
		"DARB_DATABASE_DRIVER=sqlite",
		fmt.Sprintf("DARB_LOCAL_STATE_FILE=%s", paths.StateFile),
		"DARB_SERVER_HOST=127.0.0.1", // Bind to localhost only
	)

	// Start detached process (new session so it survives CLI exit)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Don't wait - server runs in background
	// The log file handle will be inherited by the child process
	return nil
}

// waitForServerReady waits for the server to write its state file
func waitForServerReady(timeout time.Duration) (*ServerState, error) {
	paths, err := getLocalServerPaths()
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := readServerState()
		if err == nil && isProcessRunning(state.PID) {
			// Verify server is actually responding
			if checkServerHealth(state.Port) {
				return state, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return nil, fmt.Errorf("server did not become ready within %v. Check logs at %s", timeout, paths.LogFile)
}

// checkServerHealth checks if the server is responding on the given port
func checkServerHealth(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ensureLocalServer ensures a local server is running and returns connection info
func ensureLocalServer() (url string, token string, err error) {
	paths, err := getLocalServerPaths()
	if err != nil {
		return "", "", err
	}

	// Step 1: Check if server.state exists and process is alive
	state, err := readServerState()
	if err == nil && isProcessRunning(state.PID) && checkServerHealth(state.Port) {
		// Server is running, return connection info
		return fmt.Sprintf("http://127.0.0.1:%d", state.Port), state.Token, nil
	}

	// Step 2: Server not running, acquire lock
	lock, err := acquireSpawnLock()
	if err != nil {
		return "", "", err
	}
	defer releaseSpawnLock(lock)

	// Step 3: Double-check (another process may have spawned while we waited)
	state, err = readServerState()
	if err == nil && isProcessRunning(state.PID) && checkServerHealth(state.Port) {
		return fmt.Sprintf("http://127.0.0.1:%d", state.Port), state.Token, nil
	}

	// Step 4: Clean up stale state file if exists
	os.Remove(paths.StateFile)

	// Step 5: Find available port
	port, err := findAvailablePort(8460)
	if err != nil {
		return "", "", err
	}

	// Step 6: Spawn server process
	fmt.Fprintf(os.Stderr, "Starting local server on port %d...\n", port)
	if err := spawnLocalServer(port); err != nil {
		return "", "", err
	}

	// Step 7: Wait for server to be ready
	state, err = waitForServerReady(30 * time.Second)
	if err != nil {
		return "", "", err
	}

	fmt.Fprintf(os.Stderr, "Local server started (PID: %d)\n", state.PID)
	return fmt.Sprintf("http://127.0.0.1:%d", state.Port), state.Token, nil
}

// getLocalServerStatus returns the current status of the local server
func getLocalServerStatus() (status string, port int, pid int, uptime time.Duration) {
	state, err := readServerState()
	if err != nil {
		return "not running", 0, 0, 0
	}

	if !isProcessRunning(state.PID) {
		return "not running (stale)", 0, 0, 0
	}

	if !checkServerHealth(state.Port) {
		return "not responding", state.Port, state.PID, time.Since(state.StartedAt)
	}

	return "running", state.Port, state.PID, time.Since(state.StartedAt)
}
