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
	"github.com/nebari-dev/nebi/internal/server"
	"github.com/nebari-dev/nebi/internal/store"
)

// e2eEnv holds the test environment state.
var e2eEnv struct {
	serverURL string
	token     string
	dataDir   string
	configDir string
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
	port, err := findFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: failed to find free port: %v\n", err)
		os.Exit(1)
	}

	// Create temp dirs
	dbDir, _ := os.MkdirTemp("", "nebi-e2e-db-*")
	defer os.RemoveAll(dbDir)
	dbPath := filepath.Join(dbDir, "e2e.db")

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

	os.Setenv("NEBI_SERVER_PORT", fmt.Sprintf("%d", port))
	os.Setenv("NEBI_DATABASE_DSN", dbPath)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx, server.Config{
			Port:    port,
			Mode:    "both",
			Version: "e2e-test",
		})
	}()

	e2eEnv.serverURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealth(e2eEnv.serverURL+"/api/v1/health", serverErr, origStderr)

	// Restore stdout/stderr for test output
	os.Stdout = origStdout
	os.Stderr = origStderr
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Login to server
	client := cliclient.NewWithoutAuth(e2eEnv.serverURL)
	loginResp, err := client.Login(context.Background(), "admin", "adminpass")
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: login failed: %v\n", err)
		os.Exit(1)
	}
	e2eEnv.token = loginResp.Token

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
	// pull.go
	pullOutput = "."
	pullForce = false
	// push.go
	pushForce = false
	// workspace.go
	wsListRemote = false
	wsRemoveRemote = false
	// login.go
	loginToken = ""
	// publish.go
	publishRegistry = ""
	publishTag = ""
	publishRepo = ""
	// registry.go
	registryAddName = ""
	registryAddURL = ""
	registryAddUsername = ""
	registryAddNamespace = ""
	registryAddDefault = false
	registryAddPwdStdin = false
	registryRemoveForce = false
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

// runCLIWithStdin runs the CLI with the given args and stdin input.
func runCLIWithStdin(t *testing.T, cwd, stdin string, args ...string) runResult {
	t.Helper()
	origStdin := os.Stdin

	// Create a pipe for stdin
	stdinR, stdinW, _ := os.Pipe()
	os.Stdin = stdinR
	go func() {
		stdinW.WriteString(stdin)
		stdinW.Close()
	}()

	result := runCLI(t, cwd, args...)

	os.Stdin = origStdin
	stdinR.Close()

	return result
}

// writePixiFiles writes pixi.toml and pixi.lock in the given directory.
func writePixiFiles(t *testing.T, dir, toml, lock string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(toml), 0644)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte(lock), 0644)
}

// setupLocalStore configures the store to use test-specific directories
// and pre-registers the e2e server with credentials.
// Each test gets its own data dir to avoid shared state between tests.
func setupLocalStore(t *testing.T) {
	t.Helper()

	dataDir := t.TempDir()
	os.Setenv("NEBI_DATA_DIR", dataDir)

	s, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	if err := s.SaveServerURL(e2eEnv.serverURL); err != nil {
		t.Fatalf("failed to save server URL: %v", err)
	}
	if err := s.SaveCredentials(&store.Credentials{
		Token:    e2eEnv.token,
		Username: "admin",
	}); err != nil {
		t.Fatalf("failed to save credentials: %v", err)
	}
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

	res := runCLI(t, srcDir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Modify local pixi.toml
	localToml := serverToml + "\n[dependencies]\nnumpy = \"*\"\n"
	os.WriteFile(filepath.Join(srcDir, "pixi.toml"), []byte(localToml), 0644)

	// Diff against server version
	res = runCLI(t, srcDir, "diff", wsName+":"+tag)
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
	res := runCLI(t, srcDir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Pushed") {
		t.Errorf("push output missing 'Pushed': stdout=%s stderr=%s", res.Stdout, res.Stderr)
	}

	// Pull to new directory
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", wsName+":"+tag)
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

	res := runCLI(t, dir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Creating") {
		t.Errorf("expected auto-create message, got stderr: %s", res.Stderr)
	}
}

func TestE2E_PushWithoutTag(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"notag\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	// Push without a tag should succeed (auto-tags with content hash + latest)
	res := runCLI(t, dir, "push", "notag-ws")
	if res.ExitCode != 0 {
		t.Fatalf("push without tag should succeed, got exit %d:\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify auto-generated tags in output
	if !strings.Contains(res.Stderr, "sha-") {
		t.Errorf("expected content hash tag in output, got: %s", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "latest") {
		t.Errorf("expected latest tag in output, got: %s", res.Stderr)
	}
}

func TestE2E_PushRequiresPixiToml(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir() // empty

	res := runCLI(t, dir, "push", "some-ws:v1")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when pixi.toml missing")
	}
}

func TestE2E_PullNotFound(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()
	res := runCLI(t, dir, "pull", "nonexistent-ws:v1")
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
	res := runCLI(t, srcDir, "push", wsName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("setup push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull nonexistent tag
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", wsName+":nonexistent")
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

	res := runCLI(t, srcDir, "push", wsName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("push v1 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Push v2
	toml2 := "[project]\nname = \"multi-v2\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\nscipy = \"*\"\n"
	lock2 := "version: 6\npackages:\n  - name: scipy\n    version: \"1.12\"\n"
	writePixiFiles(t, srcDir, toml2, lock2)

	res = runCLI(t, srcDir, "push", wsName+":v2")
	if res.ExitCode != 0 {
		t.Fatalf("push v2 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull v1 and verify
	dir1 := t.TempDir()
	res = runCLI(t, dir1, "pull", wsName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("pull v1 failed: %s %s", res.Stdout, res.Stderr)
	}
	got1, _ := os.ReadFile(filepath.Join(dir1, "pixi.toml"))
	if string(got1) != toml1 {
		t.Errorf("v1 pixi.toml mismatch:\ngot:  %s\nwant: %s", got1, toml1)
	}

	// Pull v2 and verify
	dir2 := t.TempDir()
	res = runCLI(t, dir2, "pull", wsName+":v2")
	if res.ExitCode != 0 {
		t.Fatalf("pull v2 failed: %s %s", res.Stdout, res.Stderr)
	}
	got2, _ := os.ReadFile(filepath.Join(dir2, "pixi.toml"))
	if string(got2) != toml2 {
		t.Errorf("v2 pixi.toml mismatch:\ngot:  %s\nwant: %s", got2, toml2)
	}
}

func TestE2E_LoginWithToken(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	res := runCLI(t, dir, "login", e2eEnv.serverURL, "--token", "fake-token-123")
	if res.ExitCode != 0 {
		t.Fatalf("login failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Logged in") {
		t.Errorf("expected 'Logged in' message, got stderr: %s", res.Stderr)
	}
}

func TestE2E_WorkspaceRemove(t *testing.T) {
	setupLocalStore(t)

	// Create and init a workspace
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"remove-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	wsName := filepath.Base(dir)

	// Remove workspace by name
	res = runCLI(t, dir, "workspace", "remove", wsName)
	if res.ExitCode != 0 {
		t.Fatalf("remove failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "untouched") {
		t.Errorf("expected 'untouched' message, got: %s", res.Stderr)
	}

	// Should no longer appear in list
	res = runCLI(t, dir, "workspace", "list")
	if strings.Contains(res.Stdout, wsName) {
		t.Errorf("removed workspace still in list: %s", res.Stdout)
	}

	// pixi.toml should still exist (remove only untracks, doesn't delete files)
	if _, err := os.Stat(filepath.Join(dir, "pixi.toml")); err != nil {
		t.Error("pixi.toml was deleted during workspace remove")
	}

	// Re-init, then remove by path
	res = runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("re-init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "workspace", "remove", dir)
	if res.ExitCode != 0 {
		t.Fatalf("remove by path failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "untouched") {
		t.Errorf("expected 'untouched' message for path remove, got: %s", res.Stderr)
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

	res := runCLI(t, srcDir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Verify it exists on the server
	res = runCLI(t, srcDir, "workspace", "list", "--remote")
	if res.ExitCode != 0 {
		t.Fatalf("workspace list failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, wsName) {
		t.Fatalf("expected %s in server workspace list, got: %s", wsName, res.Stdout)
	}

	// Remove from server
	res = runCLI(t, srcDir, "workspace", "remove", wsName, "--remote")
	if res.ExitCode != 0 {
		t.Fatalf("workspace remove --remote failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Deleted") {
		t.Errorf("expected 'Deleted' message, got stderr: %s", res.Stderr)
	}

	// Wait for async deletion to complete, then verify it's gone
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res = runCLI(t, srcDir, "workspace", "list", "--remote")
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

	// Create and init a workspace
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"alias-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	wsName := filepath.Base(dir)

	// Use 'rm' alias to remove by name
	res = runCLI(t, dir, "workspace", "rm", wsName)
	if res.ExitCode != 0 {
		t.Fatalf("workspace rm (alias) failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify it's gone
	res = runCLI(t, dir, "workspace", "list")
	if strings.Contains(res.Stdout, wsName) {
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

func TestE2E_RegistryListDefault(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	// Fresh server should have the default nebari-environments registry
	res := runCLI(t, dir, "registry", "list")
	if res.ExitCode != 0 {
		t.Fatalf("registry list failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "nebari-environments") {
		t.Errorf("expected default 'nebari-environments' registry, got stdout: %s", res.Stdout)
	}
}

func TestE2E_RegistryAdd(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Add a registry
	res := runCLI(t, dir, "registry", "add", "--name", "test-registry", "--url", "ghcr.io", "--namespace", "testns")
	if res.ExitCode != 0 {
		t.Fatalf("registry add failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Added registry") {
		t.Errorf("expected 'Added registry' message, got stderr: %s", res.Stderr)
	}

	// Verify it shows up in list
	res = runCLI(t, dir, "registry", "list")
	if res.ExitCode != 0 {
		t.Fatalf("registry list failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "test-registry") {
		t.Errorf("expected registry in list, got stdout: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "ghcr.io") {
		t.Errorf("expected URL in list, got stdout: %s", res.Stdout)
	}
}

func TestE2E_RegistryAddDuplicate(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Add first registry
	res := runCLI(t, dir, "registry", "add", "--name", "dup-test", "--url", "ghcr.io", "--namespace", "testns")
	if res.ExitCode != 0 {
		t.Fatalf("first add failed: %s", res.Stderr)
	}

	// Try to add with same name - should fail
	res = runCLI(t, dir, "registry", "add", "--name", "dup-test", "--url", "quay.io", "--namespace", "testns")
	if res.ExitCode == 0 {
		t.Fatal("expected error for duplicate name")
	}
	// Server may return different error messages, just verify we got an error
	if !strings.Contains(res.Stderr, "Error") && !strings.Contains(res.Stderr, "error") {
		t.Errorf("expected error message, got: %s", res.Stderr)
	}
}

func TestE2E_RegistryAddWithPasswordStdin(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Add with password via stdin
	res := runCLIWithStdin(t, dir, "secret-password\n", "registry", "add",
		"--name", "auth-registry",
		"--url", "private.registry.io",
		"--username", "testuser",
		"--namespace", "testns",
		"--password-stdin")
	if res.ExitCode != 0 {
		t.Fatalf("registry add with password failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify it shows up in list
	res = runCLI(t, dir, "registry", "list")
	if !strings.Contains(res.Stdout, "auth-registry") {
		t.Errorf("expected registry in list, got stdout: %s", res.Stdout)
	}
}

func TestE2E_RegistryRemove(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Add a registry first
	res := runCLI(t, dir, "registry", "add", "--name", "remove-me", "--url", "example.io", "--namespace", "testns")
	if res.ExitCode != 0 {
		t.Fatalf("add failed: %s", res.Stderr)
	}

	// Remove it with --force
	res = runCLI(t, dir, "registry", "remove", "remove-me", "--force")
	if res.ExitCode != 0 {
		t.Fatalf("registry remove failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Removed registry") {
		t.Errorf("expected 'Removed registry' message, got stderr: %s", res.Stderr)
	}

	// Verify it's gone from list
	res = runCLI(t, dir, "registry", "list")
	if strings.Contains(res.Stdout, "remove-me") {
		t.Errorf("registry should be removed, but still in list: %s", res.Stdout)
	}
}

func TestE2E_RegistryRemoveNotFound(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	res := runCLI(t, dir, "registry", "remove", "nonexistent", "--force")
	if res.ExitCode == 0 {
		t.Fatal("expected error for nonexistent registry")
	}
	if !strings.Contains(res.Stderr, "not found") {
		t.Errorf("expected 'not found' error, got: %s", res.Stderr)
	}
}

func TestE2E_RegistryRemoveConfirmNo(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Add a registry first
	res := runCLI(t, dir, "registry", "add", "--name", "keep-me", "--url", "example.io", "--namespace", "testns")
	if res.ExitCode != 0 {
		t.Fatalf("add failed: %s", res.Stderr)
	}

	// Try remove without --force, stdin provides empty (defaults to no)
	res = runCLIWithStdin(t, dir, "\n", "registry", "remove", "keep-me")
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0 when declining, got %d: %s", res.ExitCode, res.Stderr)
	}

	// Registry should still exist
	res = runCLI(t, dir, "registry", "list")
	if !strings.Contains(res.Stdout, "keep-me") {
		t.Errorf("registry should still exist after declining remove: %s", res.Stdout)
	}
}

func TestE2E_RegistryRemoveConfirmYes(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Add a registry first
	res := runCLI(t, dir, "registry", "add", "--name", "confirm-remove", "--url", "example.io", "--namespace", "testns")
	if res.ExitCode != 0 {
		t.Fatalf("add failed: %s", res.Stderr)
	}

	// Remove with confirmation
	res = runCLIWithStdin(t, dir, "y\n", "registry", "remove", "confirm-remove")
	if res.ExitCode != 0 {
		t.Fatalf("registry remove failed (exit %d):\nstderr: %s", res.ExitCode, res.Stderr)
	}

	// Registry should be gone
	res = runCLI(t, dir, "registry", "list")
	if strings.Contains(res.Stdout, "confirm-remove") {
		t.Errorf("registry should be removed: %s", res.Stdout)
	}
}

func TestE2E_WorkspacePublishNotFound(t *testing.T) {
	setupLocalStore(t)

	dir := t.TempDir()

	res := runCLI(t, dir, "publish", "nonexistent")
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

	res := runCLI(t, srcDir, "push", wsName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Publish should fail because no registry is configured
	res = runCLI(t, srcDir, "publish", wsName+":v1")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when no registry configured")
	}
}

func TestE2E_DiffByWorkspaceName(t *testing.T) {
	setupLocalStore(t)

	// Create and init a workspace (name comes from directory basename)
	dir := t.TempDir()
	toml := "[project]\nname = \"diff-name-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	writePixiFiles(t, dir, toml, "version: 6\n")

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	wsName := filepath.Base(dir)

	// Create a second directory with different content
	dir2 := t.TempDir()
	toml2 := "[project]\nname = \"diff-name-other\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	writePixiFiles(t, dir2, toml2, "version: 6\n")

	// Diff using tracked workspace name vs current directory
	res = runCLI(t, dir2, "diff", wsName)
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
	res = runCLI(t, dir, "push", wsName+":"+tag)
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

	res = runCLI(t, dir, "push", wsName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("push v1 failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Push with :tag shorthand (should reuse workspace name from origin)
	res = runCLI(t, dir, "push", ":v2")
	if res.ExitCode != 0 {
		t.Fatalf("push :v2 failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Using workspace") {
		t.Errorf("expected 'Using workspace' message, got stderr: %s", res.Stderr)
	}
	// Content is same as v1, so it's deduplicated — check for either "Pushed" or "Content unchanged"
	hasOutput := strings.Contains(res.Stderr, wsName) && strings.Contains(res.Stderr, "v2")
	if !hasOutput {
		t.Errorf("expected push output with workspace name and tag v2, got stderr: %s", res.Stderr)
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
	res = runCLI(t, dir, "push", ":v1")
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

	res = runCLI(t, dir, "push", wsName+":"+tag)
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

	res = runCLI(t, dir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull into same dir WITHOUT --force (stdin is closed/empty, so prompt defaults to N → abort)
	res = runCLI(t, dir, "pull", wsName+":"+tag)
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

	res = runCLI(t, dir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull with --force should succeed without prompt
	res = runCLI(t, dir, "pull", wsName+":"+tag, "--force")
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

	res = runCLI(t, dir, "push", wsName+":"+tag)
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
	if !strings.Contains(res.Stdout, "No origin") {
		t.Errorf("expected 'No origin' in status, got: %s", res.Stdout)
	}

	// Push to establish origin
	wsName := "e2e-status-ws"
	res = runCLI(t, dir, "push", wsName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Status with origin
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Origin:") {
		t.Errorf("expected 'Origin:' section, got: %s", res.Stdout)
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
	res = runCLI(t, dir, "push", wsName+":v1")
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

	res = runCLI(t, srcDir, "push", wsName+":"+tag)
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

	res = runCLI(t, dstDir, "pull", wsName+":"+tag, "--force")
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

func TestE2E_ShellAutoInit(t *testing.T) {
	dataDir := t.TempDir()
	os.Setenv("NEBI_DATA_DIR", dataDir)

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
	dataDir := t.TempDir()
	os.Setenv("NEBI_DATA_DIR", dataDir)

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
	dataDir := t.TempDir()
	os.Setenv("NEBI_DATA_DIR", dataDir)

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
	dataDir := t.TempDir()
	os.Setenv("NEBI_DATA_DIR", dataDir)

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
	dataDir := t.TempDir()
	os.Setenv("NEBI_DATA_DIR", dataDir)

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
	dataDir := t.TempDir()
	os.Setenv("NEBI_DATA_DIR", dataDir)

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

func TestE2E_PullAutoTracksWorkspace(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-pull-autotrack"
	tag := "v1.0"

	// Push a workspace from a source directory
	srcDir := t.TempDir()
	toml := "[project]\nname = \"autotrack-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\npackages: []\n"
	writePixiFiles(t, srcDir, toml, lock)

	res := runCLI(t, srcDir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, srcDir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull into a fresh, untracked directory (no nebi init)
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("pull failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Status should recognize the workspace (not say "Not a tracked workspace")
	res = runCLI(t, dstDir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stderr, "Not a tracked workspace") {
		t.Error("pull into untracked directory should auto-track the workspace, but status says 'Not a tracked workspace'")
	}
	if !strings.Contains(res.Stdout, "Workspace:") {
		t.Errorf("expected 'Workspace:' in status output, got stdout: %s stderr: %s", res.Stdout, res.Stderr)
	}

	// Origin should also be saved
	if !strings.Contains(res.Stdout, wsName+":"+tag) {
		t.Errorf("expected origin %s:%s in status output, got: %s", wsName, tag, res.Stdout)
	}
	if !strings.Contains(res.Stdout, "pull") {
		t.Errorf("expected 'pull' action in status output, got: %s", res.Stdout)
	}
}

func TestE2E_PushAutoTracksWorkspace(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-push-autotrack"
	tag := "v1.0"

	// Create a workspace directory with pixi files but do NOT run nebi init
	dir := t.TempDir()
	toml := "[project]\nname = \"autotrack-push\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\npackages: []\n"
	writePixiFiles(t, dir, toml, lock)

	// Push from the untracked directory — should auto-track
	res := runCLI(t, dir, "push", wsName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Status should recognize the workspace (not say "Not a tracked workspace")
	res = runCLI(t, dir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stderr, "Not a tracked workspace") {
		t.Error("push from untracked directory should auto-track the workspace, but status says 'Not a tracked workspace'")
	}
	if !strings.Contains(res.Stdout, "Workspace:") {
		t.Errorf("expected 'Workspace:' in status output, got stdout: %s stderr: %s", res.Stdout, res.Stderr)
	}

	// Origin should also be saved
	if !strings.Contains(res.Stdout, wsName+":"+tag) {
		t.Errorf("expected origin %s:%s in status output, got: %s", wsName, tag, res.Stdout)
	}
	if !strings.Contains(res.Stdout, "push") {
		t.Errorf("expected 'push' action in status output, got: %s", res.Stdout)
	}
}

func TestE2E_PullWithoutTagResolvesTag(t *testing.T) {
	setupLocalStore(t)

	wsName := "e2e-pull-resolve-tag"

	// Push two tagged versions
	srcDir := t.TempDir()
	toml1 := "[project]\nname = \"resolve-tag\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock1 := "version: 6\n"
	writePixiFiles(t, srcDir, toml1, lock1)

	res := runCLI(t, srcDir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, srcDir, "push", wsName+":v1.0")
	if res.ExitCode != 0 {
		t.Fatalf("push v1.0 failed: %s %s", res.Stdout, res.Stderr)
	}

	toml2 := toml1 + "\n[dependencies]\nnumpy = \"*\"\n"
	os.WriteFile(filepath.Join(srcDir, "pixi.toml"), []byte(toml2), 0644)

	res = runCLI(t, srcDir, "push", wsName+":v2.0")
	if res.ExitCode != 0 {
		t.Fatalf("push v2.0 failed: %s %s", res.Stdout, res.Stderr)
	}

	// Pull WITHOUT a tag — should resolve to latest version and find its tag
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", wsName)
	if res.ExitCode != 0 {
		t.Fatalf("pull failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Status should show the resolved tag (v2.0), not an empty tag
	res = runCLI(t, dstDir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, wsName+":v2.0") {
		t.Errorf("expected resolved tag v2.0 in status output, got: %s", res.Stdout)
	}
}
