package localserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// ServerInfo contains the connection details for a running local server.
type ServerInfo struct {
	Port  int
	Token string
}

// URL returns the base URL for the local server.
func (s *ServerInfo) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.Port)
}

// EnsureRunning ensures a local nebi server is running.
// If a server is already running (based on server.state), it returns the connection info.
// If not, it spawns a new server and waits for it to become ready.
func EnsureRunning(ctx context.Context) (*ServerInfo, error) {
	// First, check if there's an existing server running.
	state, err := ReadState()
	if err != nil {
		slog.Debug("Failed to read server state", "error", err)
	}

	if state != nil && IsProcessAlive(state.PID) {
		// Verify the server is actually responding.
		if isServerHealthy(ctx, state.Port, state.Token) {
			slog.Debug("Local server is already running", "pid", state.PID, "port", state.Port)
			return &ServerInfo{
				Port:  state.Port,
				Token: state.Token,
			}, nil
		}
		// Process is alive but not responding; it may still be starting.
		// Give it a moment and check again.
		time.Sleep(500 * time.Millisecond)
		if isServerHealthy(ctx, state.Port, state.Token) {
			return &ServerInfo{
				Port:  state.Port,
				Token: state.Token,
			}, nil
		}
		slog.Debug("Server process alive but not responding, will spawn new instance")
	}

	// Need to spawn a new server. Acquire the lock first.
	lock, err := AcquireLock()
	if err != nil {
		// Another process is already spawning. Wait for it.
		slog.Debug("Another process is spawning the server, waiting...")
		return waitForServer(ctx)
	}
	defer lock.Release()

	// Double-check state after acquiring lock (another process may have just finished spawning).
	state, err = ReadState()
	if err == nil && state != nil && IsProcessAlive(state.PID) {
		if isServerHealthy(ctx, state.Port, state.Token) {
			return &ServerInfo{
				Port:  state.Port,
				Token: state.Token,
			}, nil
		}
	}

	// Find an available port.
	port, err := FindAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find available port: %w", err)
	}

	// Generate a token for authentication.
	token, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Get database path.
	dbPath, err := GetDBPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get database path: %w", err)
	}

	// Spawn the server process.
	nebiPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to find nebi executable: %w", err)
	}

	cmd := exec.CommandContext(ctx, nebiPath, "serve", "--port", fmt.Sprintf("%d", port))
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("DARB_SERVER_PORT=%d", port),
		"DARB_DATABASE_DRIVER=sqlite",
		fmt.Sprintf("DARB_DATABASE_DSN=%s", dbPath),
		fmt.Sprintf("DARB_AUTH_JWT_SECRET=%s", token),
		"DARB_QUEUE_TYPE=memory",
		fmt.Sprintf("NEBI_LOCAL_TOKEN=%s", token),
	)

	// Detach the process so it survives after the CLI exits.
	cmd.SysProcAttr = getSysProcAttr()

	// Redirect stdout/stderr to null to avoid blocking.
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	pid := cmd.Process.Pid
	slog.Debug("Server process started", "pid", pid, "port", port)

	// Write state file immediately (server will overwrite with final state once ready).
	state = &ServerState{
		PID:       pid,
		Port:      port,
		Token:     token,
		StartedAt: time.Now(),
	}
	if err := WriteState(state); err != nil {
		// Kill the spawned process since we can't write state.
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to write server state: %w", err)
	}

	// Detach from the child process so it doesn't become a zombie.
	cmd.Process.Release()

	// Wait for the server to become healthy.
	info := &ServerInfo{
		Port:  port,
		Token: token,
	}

	if err := waitForHealthy(ctx, port, token); err != nil {
		return nil, fmt.Errorf("server failed to become ready: %w", err)
	}

	return info, nil
}

// waitForServer waits for another process to finish spawning the server.
func waitForServer(ctx context.Context) (*ServerInfo, error) {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timed out waiting for server to start")
		case <-ticker.C:
			state, err := ReadState()
			if err != nil {
				continue
			}
			if state != nil && IsProcessAlive(state.PID) {
				if isServerHealthy(ctx, state.Port, state.Token) {
					return &ServerInfo{
						Port:  state.Port,
						Token: state.Token,
					}, nil
				}
			}
		}
	}
}

// waitForHealthy polls the server health endpoint until it responds.
func waitForHealthy(ctx context.Context, port int, token string) error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("server did not become healthy within 30 seconds")
		case <-ticker.C:
			if isServerHealthy(ctx, port, token) {
				return nil
			}
		}
	}
}

// isServerHealthy checks if the server is responding to health checks.
func isServerHealthy(ctx context.Context, port int, token string) bool {
	url := fmt.Sprintf("http://localhost:%d/api/v1/health", port)

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
