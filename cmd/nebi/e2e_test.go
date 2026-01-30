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
	serverURL string
	token     string
	dataDir   string
	configDir string
}

func TestMain(m *testing.M) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: failed to find free port: %v\n", err)
		os.Exit(1)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create temp dirs
	dbDir, _ := os.MkdirTemp("", "nebi-e2e-db-*")
	defer os.RemoveAll(dbDir)
	dbPath := filepath.Join(dbDir, "e2e.db")

	e2eEnv.dataDir, _ = os.MkdirTemp("", "nebi-e2e-data-*")
	e2eEnv.configDir, _ = os.MkdirTemp("", "nebi-e2e-config-*")

	// Set server env vars
	os.Setenv("NEBI_SERVER_PORT", fmt.Sprintf("%d", port))
	os.Setenv("NEBI_DATABASE_DRIVER", "sqlite")
	os.Setenv("NEBI_DATABASE_DSN", dbPath)
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

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx, server.Config{
			Port:    port,
			Mode:    "both",
			Version: "e2e-test",
		})
	}()

	// Wait for health endpoint
	e2eEnv.serverURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	healthURL := e2eEnv.serverURL + "/api/v1/health"
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case err := <-serverErr:
			fmt.Fprintf(origStderr, "E2E: server exited early: %v\n", err)
			os.Exit(1)
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Restore stdout/stderr for test output
	os.Stdout = origStdout
	os.Stderr = origStderr
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Login to get a token
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

// runCLI executes a CLI command in-process and captures output.
func runCLI(t *testing.T, workDir string, args ...string) runResult {
	t.Helper()

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
func setupLocalStore(t *testing.T) {
	t.Helper()

	// Point localstore at test dirs
	os.Setenv("NEBI_DATA_DIR", e2eEnv.dataDir)
	os.Setenv("NEBI_CONFIG_DIR", e2eEnv.configDir)

	// Register server in index
	store := localstore.NewStoreWithDir(e2eEnv.dataDir)
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
	localstore.SaveCredentialsTo(filepath.Join(e2eEnv.dataDir, "credentials.json"), creds)

	// Set default server in config
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
	if strings.Contains(res.Stdout, "pixi.lock") {
		t.Errorf("should not show lock diff without --lock, got: %s", res.Stdout)
	}

	// With --lock, should also show lock diff
	res = runCLI(t, dir1, "diff", dir2, "--lock")
	if res.ExitCode != 0 {
		t.Fatalf("diff --lock failed: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "extra") {
		t.Errorf("expected lock diff with --lock, got: %s", res.Stdout)
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

	// Remove local workspace (should keep files)
	baseName := filepath.Base(dir)
	res = runCLI(t, dir, "workspace", "remove", baseName)
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

func TestE2E_DiffByWorkspaceName(t *testing.T) {
	setupLocalStore(t)

	// Create and init a workspace
	dir := t.TempDir()
	toml := "[project]\nname = \"diff-name-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	writePixiFiles(t, dir, toml, "version: 6\n")

	res := runCLI(t, dir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed: %s %s", res.Stdout, res.Stderr)
	}

	// Create a second directory with different content
	dir2 := t.TempDir()
	toml2 := "[project]\nname = \"diff-name-other\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	writePixiFiles(t, dir2, toml2, "version: 6\n")

	// Diff using workspace name vs directory path
	baseName := filepath.Base(dir)
	res = runCLI(t, dir2, "diff", baseName)
	if res.ExitCode != 0 {
		t.Fatalf("diff by name failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "diff-name-test") && !strings.Contains(res.Stdout, "diff-name-other") {
		t.Errorf("expected diff output with project names, got: %s", res.Stdout)
	}
}
