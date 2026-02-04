//go:build e2e

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/nebari-dev/nebi/internal/server"
)

// e2eEnv holds the test environment state.
var e2eEnv struct {
	serverURL  string
	token      string
	server2URL string
	token2     string
	dataDir    string
	configDir  string
}

// findFreePort returns a free TCP port on localhost.
func findFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port, nil
}

// waitForHealth polls the health endpoint until it responds 200 or the deadline is reached.
func waitForHealth(url string, serverErr <-chan error, w io.Writer) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case err := <-serverErr:
			fmt.Fprintf(w, "E2E: server exited early: %v\n", err)
			os.Exit(1)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestMain(m *testing.M) {
	port1, err := findFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: failed to find free port: %v\n", err)
		os.Exit(1)
	}
	port2, err := findFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: failed to find free port for server 2: %v\n", err)
		os.Exit(1)
	}

	// Create temp dirs
	dbDir, _ := os.MkdirTemp("", "nebi-e2e-db-*")
	defer os.RemoveAll(dbDir)
	dbPath1 := filepath.Join(dbDir, "e2e1.db")
	dbPath2 := filepath.Join(dbDir, "e2e2.db")

	e2eEnv.dataDir, _ = os.MkdirTemp("", "nebi-e2e-data-*")
	e2eEnv.configDir, _ = os.MkdirTemp("", "nebi-e2e-config-*")

	// Common env vars
	os.Setenv("NEBI_DATABASE_DRIVER", "sqlite")
	os.Setenv("NEBI_QUEUE_TYPE", "memory")
	os.Setenv("NEBI_AUTH_JWT_SECRET", "e2e-test-secret")
	os.Setenv("NEBI_SERVER_MODE", "test")
	os.Setenv("NEBI_LOG_LEVEL", "error")
	os.Setenv("NEBI_DATABASE_LOG_LEVEL", "silent")
	os.Setenv("ADMIN_USERNAME", "admin")
	os.Setenv("ADMIN_PASSWORD", "adminpass")

	// Suppress server logs
	origStdout := os.Stdout
	origStderr := os.Stderr
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	os.Stderr = devNull
	log.SetOutput(io.Discard)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Start server 1 ---
	os.Setenv("NEBI_SERVER_PORT", fmt.Sprintf("%d", port1))
	os.Setenv("NEBI_DATABASE_DSN", dbPath1)

	server1Err := make(chan error, 1)
	go func() {
		server1Err <- server.Run(ctx, server.Config{
			Port:    port1,
			Mode:    "both",
			Version: "e2e-test",
		})
	}()

	e2eEnv.serverURL = fmt.Sprintf("http://127.0.0.1:%d", port1)
	waitForHealth(e2eEnv.serverURL+"/api/v1/health", server1Err, origStderr)

	// --- Start server 2 (change DSN env before config.Load reads it) ---
	os.Setenv("NEBI_SERVER_PORT", fmt.Sprintf("%d", port2))
	os.Setenv("NEBI_DATABASE_DSN", dbPath2)

	server2Err := make(chan error, 1)
	go func() {
		server2Err <- server.Run(ctx, server.Config{
			Port:    port2,
			Mode:    "both",
			Version: "e2e-test-2",
		})
	}()

	e2eEnv.server2URL = fmt.Sprintf("http://127.0.0.1:%d", port2)
	waitForHealth(e2eEnv.server2URL+"/api/v1/health", server2Err, origStderr)

	// Restore stdout/stderr for test output
	os.Stdout = origStdout
	os.Stderr = origStderr
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Login to server 1
	client1 := cliclient.NewWithoutAuth(e2eEnv.serverURL)
	loginResp1, err := client1.Login(context.Background(), "admin", "adminpass")
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: login to server 1 failed: %v\n", err)
		os.Exit(1)
	}
	e2eEnv.token = loginResp1.Token

	// Login to server 2
	client2 := cliclient.NewWithoutAuth(e2eEnv.server2URL)
	loginResp2, err := client2.Login(context.Background(), "admin", "adminpass")
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: login to server 2 failed: %v\n", err)
		os.Exit(1)
	}
	e2eEnv.token2 = loginResp2.Token

	code := m.Run()

	cancel()
	os.RemoveAll(e2eEnv.dataDir)
	os.RemoveAll(e2eEnv.configDir)
	os.Exit(code)
}

// runResult holds the result of a CLI invocation.
type runResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// resetFlags resets all package-level flag variables to their zero values
// to prevent state leaking between in-process CLI invocations.
func resetFlags() {
	// diff.go
	diffLock = false
	diffServer = ""
	// pull.go
	pullServer = ""
	pullOutput = "."
	pullGlobal = ""
	pullForce = false
	// push.go
	pushServer = ""
	pushForce = false
	// workspace.go
	wsListServer = ""
	wsTagsServer = ""
	wsRemoveServer = ""
	// login.go
	loginToken = ""
	// registry.go
	regListServer = ""
}

// runCLI executes a CLI command in-process and captures output.
func runCLI(t *testing.T, workDir string, args ...string) runResult {
	t.Helper()

	// Reset all flag variables to prevent state leaking between invocations
	resetFlags()

	// Save and restore working directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", workDir, err)
	}
	defer os.Chdir(origDir)

	// Capture stdout/stderr
	origStdout := os.Stdout
	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW

	origStderr := os.Stderr
	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	var result runResult
	var stdoutBuf, stderrBuf bytes.Buffer

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(&stdoutBuf, stdoutR)
	}()
	go func() {
		defer wg.Done()
		io.Copy(&stderrBuf, stderrR)
	}()

	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		result.ExitCode = 1
	}

	stdoutW.Close()
	stderrW.Close()
	wg.Wait()
	stdoutR.Close()
	stderrR.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()
	return result
}

// writePixiFiles writes pixi.toml and pixi.lock in the given directory.
func writePixiFiles(t *testing.T, dir, toml, lock string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(toml), 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte(lock), 0644)
}

// setupLocalStore configures the localstore to use test-specific directories
// and pre-registers the e2e server with credentials.
// Each test gets its own data dir to avoid shared state between tests.
func setupLocalStore(t *testing.T) {
	t.Helper()

	// Create a fresh data dir for each test to avoid shared state
	dataDir := t.TempDir()

	// Point localstore at test dirs
	os.Setenv("NEBI_DATA_DIR", dataDir)
	os.Setenv("NEBI_CONFIG_DIR", e2eEnv.configDir)

	// Register server in index
	store := localstore.NewStoreWithDir(dataDir)
	idx, _ := store.LoadIndex()
	idx.Servers["e2e"] = e2eEnv.serverURL
	store.SaveIndex(idx)

	// Store credentials (now in data dir per XDG spec)
	creds := &localstore.Credentials{
		Servers: map[string]*localstore.ServerCredential{
			e2eEnv.serverURL: {
				Token:    e2eEnv.token,
				Username: "admin",
			},
		},
	}
	localstore.SaveCredentialsTo(filepath.Join(dataDir, "credentials.json"), creds)

	// Set default server in config
	cfg := &localstore.Config{DefaultServer: "e2e"}
	localstore.SaveConfig(cfg)
}

// setupLocalStoreTwoServers configures the localstore with both e2e servers.
// "e2e" is the default server (server 1), "e2e2" is server 2.
func setupLocalStoreTwoServers(t *testing.T) {
	t.Helper()

	dataDir := t.TempDir()

	os.Setenv("NEBI_DATA_DIR", dataDir)
	os.Setenv("NEBI_CONFIG_DIR", e2eEnv.configDir)

	store := localstore.NewStoreWithDir(dataDir)
	idx, _ := store.LoadIndex()
	idx.Servers["e2e"] = e2eEnv.serverURL
	idx.Servers["e2e2"] = e2eEnv.server2URL
	store.SaveIndex(idx)

	creds := &localstore.Credentials{
		Servers: map[string]*localstore.ServerCredential{
			e2eEnv.serverURL: {
				Token:    e2eEnv.token,
				Username: "admin",
			},
			e2eEnv.server2URL: {
				Token:    e2eEnv.token2,
				Username: "admin",
			},
		},
	}
	localstore.SaveCredentialsTo(filepath.Join(dataDir, "credentials.json"), creds)

	cfg := &localstore.Config{DefaultServer: "e2e"}
	localstore.SaveConfig(cfg)
}

// --- E2E Tests ---

func TestE2E_InitAndWorkspaceList(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"test-init\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	// Init
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Workspace list should show it
	res = runCLI(t, dir, "workspace", "list")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, filepath.Base(dir)) {
		t.Errorf("expected workspace name in list, got: %s", res.Stdout)
	}
}

func TestE2E_DiffLocalDir(t *testing.T) {
	setupLocalStore(t)

	// Create two directories with different pixi.toml
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	toml1 := "[project]\nname = \"project-a\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	toml2 := "[project]\nname = \"project-b\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"

	writePixiFiles(t, dir1, toml1, lock)
	writePixiFiles(t, dir2, toml2, lock+"extra: true\n")

	// Diff dir2 against dir1 (run from dir1)
	res := runCLI(t, dir1, "diff", dir2)
	if res.ExitCode != 0 {
		t.Fatalf("diff failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	// Should show pixi.toml diff but not pixi.lock (no --lock)
	if !strings.Contains(res.Stdout, "project-a") || !strings.Contains(res.Stdout, "project-b") {
		t.Errorf("expected pixi.toml diff output, got: %s", res.Stdout)
	}
	// Without --lock, a hint about lock changes may appear but not full lock details
	if strings.Contains(res.Stdout, "no package changes") || strings.Contains(res.Stdout, "packages:") {
		t.Errorf("should not show full lock diff without --lock, got: %s", res.Stdout)
	}

	// With --lock, should also show lock diff section
	res = runCLI(t, dir1, "diff", dir2, "--lock")
	if res.ExitCode != 0 {
		t.Fatalf("diff --lock failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "pixi.lock") {
		t.Errorf("expected pixi.lock section with --lock, got: %s", res.Stdout)
	}

	// Two explicit directory args (run from unrelated dir)
	tmpDir := t.TempDir()
	res = runCLI(t, tmpDir, "diff", dir1, dir2)
	if res.ExitCode != 0 {
		t.Fatalf("diff two-arg failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "project-a") || !strings.Contains(res.Stdout, "project-b") {
		t.Errorf("expected pixi.toml diff in two-arg mode, got: %s", res.Stdout)
	}
}

func TestE2E_DiffAgainstServer(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-diff-server"
	tag := "v1.0"

	// Create and push a version
	srcDir := t.TempDir()
	serverToml := "[project]\nname = \"diff-server\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	serverLock := "version: 6\npackages: []\n"
	writePixiFiles(t, srcDir, serverToml, serverLock)

	res := runCLI(t, srcDir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Modify local pixi.toml
	localToml := serverToml + "\n[dependencies]\nnumpy = \"*\"\n"
	os.WriteFile(filepath.Join(srcDir, "pixi.toml"), []byte(localToml), 0644)

	// Diff against server version
	res = runCLI(t, srcDir, "diff", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("diff failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "numpy") {
		t.Errorf("expected diff to show numpy change, got: %s", res.Stdout)
	}
}

func TestE2E_PushAndPull(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-pushpull"
	tag := "v1.0"

	// Create source with pixi files
	srcDir := t.TempDir()
	toml := "[project]\nname = \"push-pull-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\npackages: []\n"
	writePixiFiles(t, srcDir, toml, lock)

	// Push
	res := runCLI(t, srcDir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Pushed") {
		t.Errorf("push output missing 'Pushed': stdout=%s stderr=%s", res.Stdout, res.Stderr)
	}

	// Pull to new directory
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("pull failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify content
	pulledToml, err := os.ReadFile(filepath.Join(dstDir, "pixi.toml"))
	if err != nil {
		t.Fatalf("failed to read pulled pixi.toml: %v", err)
	}
	if string(pulledToml) != toml {
		t.Errorf("pixi.toml mismatch:\ngot:  %s\nwant: %s", pulledToml, toml)
	}

	pulledLock, err := os.ReadFile(filepath.Join(dstDir, "pixi.lock"))
	if err != nil {
		t.Fatalf("failed to read pulled pixi.lock: %v", err)
	}
	if string(pulledLock) != lock {
		t.Errorf("pixi.lock mismatch:\ngot:  %s\nwant: %s", pulledLock, lock)
	}
}

func TestE2E_PushAutoCreatesWorkspace(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-autocreate"
	tag := "v1.0"

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"autocreate\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Creating") {
		t.Errorf("expected auto-create message, got stderr: %s", res.Stderr)
	}
}

func TestE2E_PushRequiresTag(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"notag\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "push", "some-ws", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when tag is missing")
	}
}

func TestE2E_PushRequiresPixiToml(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir() // empty

	res := runCLI(t, dir, "push", "some-ws:v1", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when pixi.toml missing")
	}
}

func TestE2E_PullNotFound(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	res := runCLI(t, dir, "pull", "nonexistent-ws:v1", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for nonexistent workspace")
	}
}

func TestE2E_PullBadTag(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-badtag"

	// Push first
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"badtag\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)
	res := runCLI(t, srcDir, "push", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("setup push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull nonexistent tag
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", wsName+":nonexistent", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for nonexistent tag")
	}
}

func TestE2E_PushMultipleVersions(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-multi"

	srcDir := t.TempDir()

	// Push v1
	toml1 := "[project]\nname = \"multi-v1\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock1 := "version: 6\npackages:\n  - name: numpy\n    version: \"1.0\"\n"
	writePixiFiles(t, srcDir, toml1, lock1)

	res := runCLI(t, srcDir, "push", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push v1 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push v2
	toml2 := "[project]\nname = \"multi-v2\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\nscipy = \"*\"\n"
	lock2 := "version: 6\npackages:\n  - name: scipy\n    version: \"1.12\"\n"
	writePixiFiles(t, srcDir, toml2, lock2)

	res = runCLI(t, srcDir, "push", wsName+":v2", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push v2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull v1 and verify
	dir1 := t.TempDir()
	res = runCLI(t, dir1, "pull", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("pull v1 failed: %s %s", res.Stdout, res.Stderr)
	}
	got1, _ := os.ReadFile(filepath.Join(dir1, "pixi.toml"))
	if string(got1) != toml1 {
		t.Errorf("v1 pixi.toml mismatch:\ngot:  %s\nwant: %s", got1, toml1)
	}

	// Pull v2 and verify
	dir2 := t.TempDir()
	res = runCLI(t, dir2, "pull", wsName+":v2", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("pull v2 failed: %s %s", res.Stdout, res.Stderr)
	}
	got2, _ := os.ReadFile(filepath.Join(dir2, "pixi.toml"))
	if string(got2) != toml2 {
		t.Errorf("v2 pixi.toml mismatch:\ngot:  %s\nwant: %s", got2, toml2)
	}
}

func TestE2E_ServerAddListRemove(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	// Add
	res := runCLI(t, dir, "server", "add", "testsvr", "https://test.example.com")
	if res.ExitCode != 0 {
		t.Fatalf("server add failed: %s %s", res.Stdout, res.Stderr)
	}

	// List
	res = runCLI(t, dir, "server", "list")
	if res.ExitCode != 0 {
		t.Fatalf("server list failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "testsvr") {
		t.Errorf("expected 'testsvr' in list, got: %s", res.Stdout)
	}

	// Remove
	res = runCLI(t, dir, "server", "remove", "testsvr")
	if res.ExitCode != 0 {
		t.Fatalf("server remove failed: %s %s", res.Stdout, res.Stderr)
	}
}

func TestE2E_LoginWithToken(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	res := runCLI(t, dir, "login", "e2e", "--token", "fake-token-123")
	if res.ExitCode != 0 {
		t.Fatalf("login failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Logged in") {
		t.Errorf("expected 'Logged in' message, got stderr: %s", res.Stderr)
	}
}

func TestE2E_PullGlobal(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-pull-global"
	tag := "v1.0"

	// Push a workspace to the server first
	srcDir := t.TempDir()
	toml := "[project]\nname = \"global-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\npackages: []\n"
	writePixiFiles(t, srcDir, toml, lock)

	res := runCLI(t, srcDir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull as global workspace
	tmpDir := t.TempDir()
	res = runCLI(t, tmpDir, "pull", wsName+":"+tag, "--global", "my-global-env", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("pull --global failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "global workspace") {
		t.Errorf("expected 'global workspace' in output, got stderr: %s", res.Stderr)
	}

	// Workspace list should show it as global
	res = runCLI(t, tmpDir, "workspace", "list")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "my-global-env") {
		t.Errorf("expected 'my-global-env' in workspace list, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "global") {
		t.Errorf("expected 'global' type in workspace list, got: %s", res.Stdout)
	}

	// Pulling same name without --force should fail
	res = runCLI(t, tmpDir, "pull", wsName+":"+tag, "--global", "my-global-env", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when global workspace already exists without --force")
	}

	// With --force should succeed
	res = runCLI(t, tmpDir, "pull", wsName+":"+tag, "--global", "my-global-env", "--force", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("pull --global --force failed: %s %s", res.Stdout, res.Stderr)
	}
}

func TestE2E_WorkspacePromote(t *testing.T) {
	setupLocalStore(t)

	// Create and init a local workspace
	dir := t.TempDir()
	toml := "[project]\nname = \"promote-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Promote to global
	res = runCLI(t, dir, "workspace", "promote", "promoted-env")
	if res.ExitCode != 0 {
		t.Fatalf("promote failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "promoted-env") {
		t.Errorf("expected 'promoted-env' in output, got stderr: %s", res.Stderr)
	}

	// Should show up in workspace list as global
	res = runCLI(t, dir, "workspace", "list")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "promoted-env") || !strings.Contains(res.Stdout, "global") {
		t.Errorf("expected promoted-env as global in list, got: %s", res.Stdout)
	}

	// Promoting with same name should fail
	res = runCLI(t, dir, "workspace", "promote", "promoted-env")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when name already exists")
	}
}

func TestE2E_WorkspaceRemove(t *testing.T) {
	setupLocalStore(t)

	// Create and init a local workspace
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"remove-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Promote to global
	res = runCLI(t, dir, "workspace", "promote", "to-remove")
	if res.ExitCode != 0 {
		t.Fatalf("promote failed: %s %s", res.Stdout, res.Stderr)
	}

	// Remove the global workspace
	res = runCLI(t, dir, "workspace", "remove", "to-remove")
	if res.ExitCode != 0 {
		t.Fatalf("remove failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Should no longer appear in list
	res = runCLI(t, dir, "workspace", "list")
	if strings.Contains(res.Stdout, "to-remove") {
		t.Errorf("removed workspace still in list: %s", res.Stdout)
	}

	// Remove local workspace by path (should keep files)
	res = runCLI(t, dir, "workspace", "remove", dir)
	if res.ExitCode != 0 {
		t.Fatalf("remove local failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "untouched") {
		t.Errorf("expected 'untouched' message for local remove, got: %s", res.Stderr)
	}

	// pixi.toml should still exist
	if _, err := os.Stat(filepath.Join(dir, "pixi.toml")); err != nil {
		t.Error("pixi.toml was deleted for local workspace remove")
	}
}

func TestE2E_WorkspaceRemoveServer(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-remove-server"
	tag := "v1.0"

	// Push a workspace to the server
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"remove-server-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, srcDir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Verify it exists on the server
	res = runCLI(t, srcDir, "workspace", "list", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, wsName) {
		t.Fatalf("expected %s in server workspace list, got: %s", wsName, res.Stdout)
	}

	// Remove from server
	res = runCLI(t, srcDir, "workspace", "remove", wsName, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("workspace remove -s failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Deleted") {
		t.Errorf("expected 'Deleted' message, got stderr: %s", res.Stderr)
	}

	// Wait for async deletion to complete, then verify it's gone
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res = runCLI(t, srcDir, "workspace", "list", "-s", "e2e")
		if res.ExitCode != 0 {
			t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
		}
		if !strings.Contains(res.Stdout, wsName) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if strings.Contains(res.Stdout, wsName) {
		t.Errorf("workspace still on server after remove: %s", res.Stdout)
	}
}

func TestE2E_WorkspaceRemoveAlias(t *testing.T) {
	setupLocalStore(t)

	// Create and init a workspace, then promote it
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"alias-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "workspace", "promote", "alias-rm-test")
	if res.ExitCode != 0 {
		t.Fatalf("promote failed: %s %s", res.Stdout, res.Stderr)
	}

	// Use 'rm' alias to remove
	res = runCLI(t, dir, "workspace", "rm", "alias-rm-test")
	if res.ExitCode != 0 {
		t.Fatalf("workspace rm (alias) failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify it's gone
	res = runCLI(t, dir, "workspace", "list")
	if strings.Contains(res.Stdout, "alias-rm-test") {
		t.Errorf("workspace still in list after rm: %s", res.Stdout)
	}
}

func TestE2E_WorkspacePrune(t *testing.T) {
	setupLocalStore(t)

	// Create and init a workspace
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"prune-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Create a second workspace
	dir2 := t.TempDir()
	writePixiFiles(t, dir2,
		"[project]\nname = \"prune-test-2\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res = runCLI(t, dir2, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Delete one directory to simulate a missing workspace
	os.RemoveAll(dir2)

	// Workspace list should show (missing)
	res = runCLI(t, dir, "workspace", "list")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "(missing)") {
		t.Errorf("expected (missing) indicator in list, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "prune") {
		t.Errorf("expected prune hint in stderr, got: %s", res.Stderr)
	}

	// Prune should remove the missing workspace
	res = runCLI(t, dir, "workspace", "prune")
	if res.ExitCode != 0 {
		t.Fatalf("workspace prune failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Pruned") {
		t.Errorf("expected 'Pruned' message, got stderr: %s", res.Stderr)
	}

	// List should no longer show the missing workspace
	res = runCLI(t, dir, "workspace", "list")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stdout, "(missing)") {
		t.Errorf("missing workspace still in list after prune: %s", res.Stdout)
	}
}

func TestE2E_WorkspacePruneNoop(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	// Prune with nothing to prune
	res := runCLI(t, dir, "workspace", "prune")
	if res.ExitCode != 0 {
		t.Fatalf("workspace prune failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Nothing to prune") {
		t.Errorf("expected 'Nothing to prune' message, got stderr: %s", res.Stderr)
	}
}

func TestE2E_RegistryListEmpty(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	// No registries configured, should get empty message
	res := runCLI(t, dir, "registry", "list", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("registry list failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "No registries") {
		t.Errorf("expected 'No registries' message, got stderr: %s", res.Stderr)
	}
}

func TestE2E_WorkspacePublishRequiresTag(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	res := runCLI(t, dir, "workspace", "publish", "some-ws", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when tag is missing")
	}
}

func TestE2E_WorkspacePublishNotFound(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	res := runCLI(t, dir, "workspace", "publish", "nonexistent:v1", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for nonexistent workspace")
	}
}

func TestE2E_WorkspacePublishNoRegistry(t *testing.T) {
	setupLocalStore(t)

	// Push a workspace first
	wsName := "e2e-publish-noreg"
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"publish-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, srcDir, "push", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Publish should fail because no registry is configured
	res = runCLI(t, srcDir, "workspace", "publish", wsName+":v1", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when no registry configured")
	}
}

func TestE2E_DiffByWorkspaceName(t *testing.T) {
	setupLocalStore(t)

	// Create and init a workspace, then promote to global so it can be found by name
	dir := t.TempDir()
	toml := "[project]\nname = \"diff-name-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	writePixiFiles(t, dir, toml, "version: 6\n")

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "workspace", "promote", "diff-name-ws")
	if res.ExitCode != 0 {
		t.Fatalf("promote failed: %s %s", res.Stdout, res.Stderr)
	}

	// Create a second directory with different content
	dir2 := t.TempDir()
	toml2 := "[project]\nname = \"diff-name-other\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	writePixiFiles(t, dir2, toml2, "version: 6\n")

	// Diff using global workspace name vs current directory
	res = runCLI(t, dir2, "diff", "diff-name-ws")
	if res.ExitCode != 0 {
		t.Fatalf("diff by name failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "diff-name-test") && !strings.Contains(res.Stdout, "diff-name-other") {
		t.Errorf("expected diff output with project names, got: %s", res.Stdout)
	}
}

// --- Origin Tracking Tests ---

func TestE2E_PushSavesOrigin(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-origin-push"
	tag := "v1.0"

	dir := t.TempDir()
	toml := "[project]\nname = \"origin-push\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	// Init so the workspace is tracked
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push
	res = runCLI(t, dir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify origin was saved by checking status
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, wsName+":"+tag) {
		t.Errorf("expected origin %s:%s in status output, got: %s", wsName, tag, res.Stdout)
	}
	if !strings.Contains(res.Stdout, "push") {
		t.Errorf("expected 'push' action in status output, got: %s", res.Stdout)
	}
}

func TestE2E_PushColonTagShorthand(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-colon-tag"

	dir := t.TempDir()
	toml := "[project]\nname = \"colon-tag\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	// Init and push to establish origin
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "push", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push v1 failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Push with :tag shorthand (should reuse workspace name from origin)
	res = runCLI(t, dir, "push", ":v2", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push :v2 failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Using workspace") {
		t.Errorf("expected 'Using workspace' message, got stderr: %s", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Pushed "+wsName+":v2") {
		t.Errorf("expected 'Pushed %s:v2', got stderr: %s", wsName, res.Stderr)
	}
}

func TestE2E_PushColonTagNoOrigin(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"no-origin\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	// Init but don't push (no origin)
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push with :tag should fail — no origin
	res = runCLI(t, dir, "push", ":v1", "-s", "e2e")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when no origin set for :tag shorthand")
	}
}

func TestE2E_PullNoArgs(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-pull-noarg"
	tag := "v1.0"

	dir := t.TempDir()
	toml := "[project]\nname = \"pull-noarg\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	// Init and push to establish origin
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Pull with no args and --force (to skip overwrite prompt since stdin is not a tty)
	res = runCLI(t, dir, "pull", "--force")
	if res.ExitCode != 0 {
		t.Fatalf("pull (no args) failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Using origin") {
		t.Errorf("expected 'Using origin' message, got stderr: %s", res.Stderr)
	}
	if !strings.Contains(res.Stderr, wsName+":"+tag) {
		t.Errorf("expected origin ref in output, got stderr: %s", res.Stderr)
	}
}

func TestE2E_PullNoArgsNoOrigin(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"no-origin-pull\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull with no args and no origin should fail
	res = runCLI(t, dir, "pull")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when no origin set for no-arg pull")
	}
}

func TestE2E_PullOverwritePromptDefaultsNo(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-pull-prompt"
	tag := "v1.0"

	dir := t.TempDir()
	toml := "[project]\nname = \"prompt-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	// Init and push
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull into same dir WITHOUT --force (stdin is closed/empty, so prompt defaults to N → abort)
	res = runCLI(t, dir, "pull", wsName+":"+tag, "-s", "e2e")
	// Should exit 0 because abort is not an error — it prints "Aborted." and returns nil
	if !strings.Contains(res.Stderr, "Aborted") {
		t.Errorf("expected 'Aborted' when overwrite prompt defaults to no, got stderr: %s", res.Stderr)
	}
}

func TestE2E_PullForceSkipsPrompt(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-pull-force"
	tag := "v1.0"

	dir := t.TempDir()
	toml := "[project]\nname = \"force-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	// Init and push
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull with --force should succeed without prompt
	res = runCLI(t, dir, "pull", wsName+":"+tag, "-s", "e2e", "--force")
	if res.ExitCode != 0 {
		t.Fatalf("pull --force failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Pulled") {
		t.Errorf("expected 'Pulled' message, got stderr: %s", res.Stderr)
	}
}

func TestE2E_DiffNoArgs(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-diff-noarg"
	tag := "v1.0"

	dir := t.TempDir()
	toml := "[project]\nname = \"diff-noarg\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	// Init and push to establish origin
	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Diff with no args should compare local vs origin (no changes expected)
	res = runCLI(t, dir, "diff")
	if res.ExitCode != 0 {
		t.Fatalf("diff (no args) failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "No differences") {
		t.Errorf("expected 'No differences' for identical specs, got stderr: %s stdout: %s", res.Stderr, res.Stdout)
	}

	// Modify local pixi.toml and diff again — should show changes
	modifiedToml := toml + "\n[dependencies]\nnumpy = \"*\"\n"
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(modifiedToml), 0644)

	res = runCLI(t, dir, "diff")
	if res.ExitCode != 0 {
		t.Fatalf("diff (no args, with changes) failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "numpy") {
		t.Errorf("expected diff to show numpy change, got stdout: %s", res.Stdout)
	}
}

func TestE2E_DiffNoArgsNoOrigin(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"no-origin-diff\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Diff with no args and no origin should fail
	res = runCLI(t, dir, "diff")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when no origin set for no-arg diff")
	}
}

func TestE2E_Status(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	toml := "[project]\nname = \"status-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	// Status before init — should say not tracked
	res := runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Not a tracked workspace") {
		t.Errorf("expected 'Not a tracked workspace', got stderr: %s stdout: %s", res.Stderr, res.Stdout)
	}

	// Init
	res = runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Status after init (no origin)
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Workspace:") {
		t.Errorf("expected 'Workspace:' in status, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "No origins") {
		t.Errorf("expected 'No origins' in status, got: %s", res.Stdout)
	}

	// Push to establish origin
	wsName := "e2e-status-ws"
	res = runCLI(t, dir, "push", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Status with origin
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Origins:") {
		t.Errorf("expected 'Origins:' section, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, wsName+":v1") {
		t.Errorf("expected origin ref in status, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "In sync") {
		t.Errorf("expected 'In sync' status, got: %s", res.Stdout)
	}
}

func TestE2E_StatusLocalModification(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	toml := "[project]\nname = \"status-mod\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push
	wsName := "e2e-status-mod"
	res = runCLI(t, dir, "push", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Modify local pixi.toml
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(toml+"\n[dependencies]\nnumpy = \"*\"\n"), 0644)

	// Status should show local modification
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "pixi.toml modified locally") {
		t.Errorf("expected 'pixi.toml modified locally', got: %s", res.Stdout)
	}
}

func TestE2E_PullSavesOrigin(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-origin-pull"
	tag := "v1.0"

	// Push from one directory
	srcDir := t.TempDir()
	toml := "[project]\nname = \"origin-pull\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, srcDir, toml, lock)

	res := runCLI(t, srcDir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, srcDir, "push", wsName+":"+tag, "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull to a new tracked directory
	dstDir := t.TempDir()
	// Init the destination first so it's a tracked workspace
	writePixiFiles(t, dstDir, "placeholder", "placeholder")
	res = runCLI(t, dstDir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init dst failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dstDir, "pull", wsName+":"+tag, "-s", "e2e", "--force")
	if res.ExitCode != 0 {
		t.Fatalf("pull failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Status should show the pull origin
	res = runCLI(t, dstDir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, wsName+":"+tag) {
		t.Errorf("expected origin ref after pull, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "pull") {
		t.Errorf("expected 'pull' action in status, got: %s", res.Stdout)
	}
}

// --- Multi-Server E2E Tests ---

func TestE2E_MultiServerPushBothOriginsInStatus(t *testing.T) {
	setupLocalStoreTwoServers(t)

	wsName := "e2e-multi-origins"
	dir := t.TempDir()
	toml := "[project]\nname = \"multi-origins\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push to server 1
	res = runCLI(t, dir, "push", wsName+":v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push to server 2
	res = runCLI(t, dir, "push", wsName+":v1", "-s", "e2e2")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Status should show both origins
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "e2e") {
		t.Errorf("expected 'e2e' origin in status, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "e2e2") {
		t.Errorf("expected 'e2e2' origin in status, got: %s", res.Stdout)
	}
}

func TestE2E_MultiServerPullPreservesOtherOrigin(t *testing.T) {
	setupLocalStoreTwoServers(t)

	dir := t.TempDir()
	toml := "[project]\nname = \"multi-pull\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push to both servers
	res = runCLI(t, dir, "push", "e2e-multi-pull:v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "push", "e2e-multi-pull:v2", "-s", "e2e2")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull from server 2 (should update e2e2 origin but preserve e2e origin)
	res = runCLI(t, dir, "pull", "e2e-multi-pull:v2", "-s", "e2e2", "--force")
	if res.ExitCode != 0 {
		t.Fatalf("pull from e2e2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Status should show both origins
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "e2e") {
		t.Errorf("expected 'e2e' origin preserved after pull from e2e2, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "e2e2") {
		t.Errorf("expected 'e2e2' origin in status, got: %s", res.Stdout)
	}
	// e2e origin should still show push action
	// e2e2 origin should show pull action (most recent action on that server)
}

func TestE2E_MultiServerDiffNoArgsUsesDefaultServer(t *testing.T) {
	setupLocalStoreTwoServers(t)

	dir := t.TempDir()
	toml := "[project]\nname = \"multi-diff\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push to both servers
	res = runCLI(t, dir, "push", "e2e-multi-diff:v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e failed: %s %s", res.Stdout, res.Stderr)
	}
	res = runCLI(t, dir, "push", "e2e-multi-diff:v1", "-s", "e2e2")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Modify local pixi.toml
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(toml+"\n[dependencies]\nnumpy = \"*\"\n"), 0644)

	// Diff with no args should use default server (e2e)
	res = runCLI(t, dir, "diff")
	if res.ExitCode != 0 {
		t.Fatalf("diff failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	// Should show differences since we modified locally
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "numpy") {
		t.Errorf("expected diff to show numpy change, got stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
}

func TestE2E_MultiServerColonTagShorthand(t *testing.T) {
	setupLocalStoreTwoServers(t)

	dir := t.TempDir()
	toml := "[project]\nname = \"multi-colon\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push different workspace names to each server
	res = runCLI(t, dir, "push", "ws-alpha:v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push ws-alpha to e2e failed: %s %s", res.Stdout, res.Stderr)
	}
	res = runCLI(t, dir, "push", "ws-beta:v1", "-s", "e2e2")
	if res.ExitCode != 0 {
		t.Fatalf("push ws-beta to e2e2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// :v2 on e2e should reuse ws-alpha
	res = runCLI(t, dir, "push", ":v2", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push :v2 to e2e failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "ws-alpha") {
		t.Errorf("expected 'ws-alpha' from e2e origin, got stderr: %s", res.Stderr)
	}

	// :v2 on e2e2 should reuse ws-beta
	res = runCLI(t, dir, "push", ":v2", "-s", "e2e2")
	if res.ExitCode != 0 {
		t.Fatalf("push :v2 to e2e2 failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "ws-beta") {
		t.Errorf("expected 'ws-beta' from e2e2 origin, got stderr: %s", res.Stderr)
	}
}

func TestE2E_MultiServerStatusSyncPerServer(t *testing.T) {
	setupLocalStoreTwoServers(t)

	dir := t.TempDir()
	toml := "[project]\nname = \"multi-sync\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, dir, toml, lock)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push v1 to both servers
	res = runCLI(t, dir, "push", "e2e-multi-sync:v1", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e failed: %s %s", res.Stdout, res.Stderr)
	}
	res = runCLI(t, dir, "push", "e2e-multi-sync:v1", "-s", "e2e2")
	if res.ExitCode != 0 {
		t.Fatalf("push to e2e2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Both should be in sync initially
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed: %s %s", res.Stdout, res.Stderr)
	}
	output := res.Stdout
	if !strings.Contains(output, "e2e") || !strings.Contains(output, "e2e2") {
		t.Errorf("expected both origins in status, got: %s", output)
	}

	// Modify local pixi.toml — now both origins are stale
	modifiedToml := toml + "\n[dependencies]\nscipy = \"*\"\n"
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(modifiedToml), 0644)

	// Status should report local modification
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "pixi.toml modified locally") {
		t.Errorf("expected 'pixi.toml modified locally' after edit, got: %s", res.Stdout)
	}

	// Push updated version only to e2e — e2e origin hash now matches local
	res = runCLI(t, dir, "push", "e2e-multi-sync:v2", "-s", "e2e")
	if res.ExitCode != 0 {
		t.Fatalf("push v2 to e2e failed: %s %s", res.Stdout, res.Stderr)
	}

	// Verify e2e origin updated to v2, e2e2 still at v1
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	output = res.Stdout
	if !strings.Contains(output, "e2e-multi-sync:v2") {
		t.Errorf("expected e2e origin at v2, got: %s", output)
	}
	if !strings.Contains(output, "e2e-multi-sync:v1") {
		t.Errorf("expected e2e2 origin still at v1, got: %s", output)
	}
}

func TestE2E_ShellAutoInit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[project]\nname = \"auto-init-test\"\n"), 0644)

	// shell will fail because pixi isn't available in test, but ensureInit should run first
	res := runCLI(t, dir, "shell")

	// The workspace should be tracked even if pixi exec fails
	res2 := runCLI(t, dir, "workspace", "list")
	if res2.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s", res2.Stderr)
	}
	// Check either stdout or stderr for tracking message
	combined := res.Stdout + res.Stderr + res2.Stdout
	if !strings.Contains(combined, "auto-init-test") {
		t.Errorf("expected workspace to be tracked after shell auto-init, got stdout: %s, stderr: %s", res2.Stdout, res.Stderr)
	}
}

func TestE2E_RunAutoInit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[project]\nname = \"run-init-test\"\n"), 0644)

	// run will fail because pixi isn't available, but ensureInit should run
	_ = runCLI(t, dir, "run", "some-task")

	res := runCLI(t, dir, "workspace", "list")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s", res.Stderr)
	}
	wsName := filepath.Base(dir)
	if !strings.Contains(res.Stdout, wsName) {
		t.Errorf("expected workspace %q tracked after run auto-init, got: %s", wsName, res.Stdout)
	}
}

func TestE2E_ShellRejectsManifestPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[project]\nname = \"test\"\n"), 0644)

	res := runCLI(t, dir, "shell", "--manifest-path", "/some/path")
	if res.ExitCode == 0 {
		t.Error("expected shell to reject --manifest-path")
	}
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "--manifest-path cannot be used") {
		t.Errorf("expected manifest-path error, got: %s", combined)
	}
}

func TestE2E_RunRejectsManifestPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[project]\nname = \"test\"\n"), 0644)

	res := runCLI(t, dir, "run", "--manifest-path=/some/path", "task")
	if res.ExitCode == 0 {
		t.Error("expected run to reject --manifest-path")
	}
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "--manifest-path cannot be used") {
		t.Errorf("expected manifest-path error, got: %s", combined)
	}
}

func TestE2E_ShellNoPixiToml(t *testing.T) {
	dir := t.TempDir()

	res := runCLI(t, dir, "shell")
	if res.ExitCode == 0 {
		t.Error("expected shell to fail without pixi.toml")
	}
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "pixi.toml") {
		t.Errorf("expected pixi.toml error, got: %s", combined)
	}
}

func TestE2E_RunNoPixiToml(t *testing.T) {
	dir := t.TempDir()

	res := runCLI(t, dir, "run", "task")
	if res.ExitCode == 0 {
		t.Error("expected run to fail without pixi.toml")
	}
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "pixi.toml") {
		t.Errorf("expected pixi.toml error, got: %s", combined)
	}
}
