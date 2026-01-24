//go:build e2e

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/aktech/darb/internal/server"
	"github.com/google/go-containerregistry/pkg/registry"
)

// e2eEnv holds the test environment state set up by TestMain.
var e2eEnv struct {
	serverURL   string
	token       string
	configDir   string
	ociRegistry *httptest.Server
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

	// Create temp dir for SQLite database
	dbDir, err := os.MkdirTemp("", "nebi-e2e-db-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: failed to create db temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dbDir)
	dbPath := filepath.Join(dbDir, "e2e.db")

	// Set server env vars
	os.Setenv("DARB_SERVER_PORT", fmt.Sprintf("%d", port))
	os.Setenv("DARB_DATABASE_DRIVER", "sqlite")
	os.Setenv("DARB_DATABASE_DSN", dbPath)
	os.Setenv("DARB_QUEUE_TYPE", "memory")
	os.Setenv("DARB_AUTH_JWT_SECRET", "e2e-test-secret")
	os.Setenv("ADMIN_USERNAME", "admin")
	os.Setenv("ADMIN_PASSWORD", "adminpass")

	// Start server in background
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

	// Poll health endpoint until ready
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
			fmt.Fprintf(os.Stderr, "E2E: server exited early: %v\n", err)
			os.Exit(1)
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Login to get a token
	client := cliclient.NewWithoutAuth(e2eEnv.serverURL)
	loginResp, err := client.Login(context.Background(), "admin", "adminpass")
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: login failed: %v\n", err)
		os.Exit(1)
	}
	e2eEnv.token = loginResp.Token

	// Start in-memory OCI registry
	e2eEnv.ociRegistry = httptest.NewServer(registry.New())

	// Set CLI env vars
	e2eEnv.configDir, _ = os.MkdirTemp("", "nebi-e2e-config-*")
	os.Setenv("NEBI_SERVER_URL", e2eEnv.serverURL)
	os.Setenv("NEBI_TOKEN", e2eEnv.token)
	os.Setenv("NEBI_CONFIG_DIR", e2eEnv.configDir)

	// Run tests
	code := m.Run()

	// Cleanup
	cancel()
	e2eEnv.ociRegistry.Close()
	os.RemoveAll(e2eEnv.configDir)
	os.Exit(code)
}

// exitCode is a sentinel type used to intercept osExit calls.
type exitCode int

// runResult holds the result of a CLI invocation.
type runResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// runCLI executes a CLI command in-process and captures output.
func runCLI(t *testing.T, workDir string, args ...string) runResult {
	t.Helper()

	// Reset global state
	cachedConfig = nil
	apiClient = nil
	configDir = ""

	// Reset all flag variables to defaults
	resetFlags()

	// Save and restore working directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", workDir, err)
	}
	defer os.Chdir(origDir)

	// Capture stdout
	origStdout := os.Stdout
	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW

	// Capture stderr
	origStderr := os.Stderr
	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	// Override osExit to panic with our sentinel type
	origExit := osExit
	osExit = func(code int) {
		panic(exitCode(code))
	}

	var result runResult
	var stdoutBuf, stderrBuf bytes.Buffer

	// Use a WaitGroup to ensure pipe readers finish
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

	func() {
		defer func() {
			if r := recover(); r != nil {
				if code, ok := r.(exitCode); ok {
					result.ExitCode = int(code)
				} else {
					panic(r) // re-panic for unexpected panics
				}
			}
		}()

		rootCmd.SetArgs(args)
		if err := rootCmd.Execute(); err != nil {
			// cobra already printed the error; set non-zero exit
			if result.ExitCode == 0 {
				result.ExitCode = 1
			}
		}
	}()

	// Restore
	stdoutW.Close()
	stderrW.Close()
	wg.Wait()
	stdoutR.Close()
	stderrR.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr
	osExit = origExit

	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()
	return result
}

// resetFlags resets all package-level flag variables to their defaults.
func resetFlags() {
	// push.go
	pushDryRun = false
	// pull.go
	pullOutput = "."
	pullGlobal = false
	pullForce = false
	pullYes = false
	pullName = ""
	pullInstall = false
	// shell.go
	shellPixiEnv = ""
	shellGlobal = false
	shellLocal = false
	shellPath = ""
	// diff.go
	diffRemote = false
	diffJSON = false
	diffLock = false
	diffToml = false
	diffPath = "."
	// status.go
	statusRemote = false
	statusJSON = false
	statusVerbose = false
	statusPath = "."
	// serve.go
	servePort = 0
	serveMode = "both"
	// publish.go
	publishRegistry = ""
	publishAs = ""
	// repo.go
	repoListLocal = false
	repoListJSON = false
	repoInfoPath = "."
	// registry.go
	registryAddUsername = ""
	registryAddPassword = ""
	registryAddDefault = false
}

// writePixiFiles writes pixi.toml and pixi.lock in the given directory.
func writePixiFiles(t *testing.T, dir, toml, lock string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(toml), 0644); err != nil {
		t.Fatalf("failed to write pixi.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte(lock), 0644); err != nil {
		t.Fatalf("failed to write pixi.lock: %v", err)
	}
}

// --- E2E Tests ---

func TestE2E_PushAndPull(t *testing.T) {
	repoName := "e2e-push-pull-" + t.Name()
	tag := "v1.0"

	// Create source directory with pixi files
	srcDir := t.TempDir()
	toml := "[project]\nname = \"test-push-pull\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\npackages: []\n"
	writePixiFiles(t, srcDir, toml, lock)

	// Push
	res := runCLI(t, srcDir, "push", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Pushed") {
		t.Errorf("push output missing 'Pushed': %s", res.Stdout)
	}

	// Pull to new directory
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", repoName+":"+tag, "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("pull failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify content matches
	pulledToml, err := os.ReadFile(filepath.Join(dstDir, "pixi.toml"))
	if err != nil {
		t.Fatalf("failed to read pulled pixi.toml: %v", err)
	}
	if string(pulledToml) != toml {
		t.Errorf("pixi.toml content mismatch:\ngot:  %s\nwant: %s", pulledToml, toml)
	}

	pulledLock, err := os.ReadFile(filepath.Join(dstDir, "pixi.lock"))
	if err != nil {
		t.Fatalf("failed to read pulled pixi.lock: %v", err)
	}
	if string(pulledLock) != lock {
		t.Errorf("pixi.lock content mismatch:\ngot:  %s\nwant: %s", pulledLock, lock)
	}
}

func TestE2E_PushAutoCreatesRepo(t *testing.T) {
	repoName := "e2e-autocreate-" + t.Name()
	tag := "v1.0"

	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"autocreate\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\npackages: []\n",
	)

	res := runCLI(t, dir, "push", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Creating repo") {
		t.Errorf("expected auto-create message, got: %s", res.Stdout)
	}
}

func TestE2E_PushRequiresTag(t *testing.T) {
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"notag\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)

	res := runCLI(t, dir, "push", "some-repo")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when tag is missing")
	}
	if !strings.Contains(res.Stderr, "tag is required") {
		t.Errorf("expected 'tag is required' error, got: %s", res.Stderr)
	}
}

func TestE2E_PushRequiresPixiToml(t *testing.T) {
	dir := t.TempDir() // empty directory

	res := runCLI(t, dir, "push", "some-repo:v1")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit when pixi.toml missing")
	}
	if !strings.Contains(res.Stderr, "pixi.toml not found") {
		t.Errorf("expected 'pixi.toml not found' error, got: %s", res.Stderr)
	}
}

func TestE2E_PullNotFound(t *testing.T) {
	dir := t.TempDir()

	res := runCLI(t, dir, "pull", "nonexistent-repo-xyz:v1", "--yes")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for nonexistent repo")
	}
	if !strings.Contains(res.Stderr, "not found") {
		t.Errorf("expected 'not found' error, got: %s", res.Stderr)
	}
}

func TestE2E_PullBadTag(t *testing.T) {
	// First create a repo
	repoName := "e2e-badtag-" + t.Name()
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"badtag\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)
	res := runCLI(t, srcDir, "push", repoName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("setup push failed: %s", res.Stderr)
	}

	// Now try to pull a nonexistent tag
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", repoName+":nonexistent-tag", "--yes")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for nonexistent tag")
	}
	if !strings.Contains(res.Stderr, "not found") {
		t.Errorf("expected 'not found' error, got: %s", res.Stderr)
	}
}

func TestE2E_PullAlreadyUpToDate(t *testing.T) {
	repoName := "e2e-uptodate-" + t.Name()
	tag := "v1.0"

	// Push
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"uptodate\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\npackages: []\n",
	)
	res := runCLI(t, srcDir, "push", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s", res.Stderr)
	}

	// First pull
	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", repoName+":"+tag, "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("first pull failed: %s", res.Stderr)
	}

	// Second pull (should be up to date)
	res = runCLI(t, dstDir, "pull", repoName+":"+tag, "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("second pull failed (exit %d): %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Already up to date") {
		t.Errorf("expected 'Already up to date', got: %s", res.Stdout)
	}
}

func TestE2E_StatusClean(t *testing.T) {
	repoName := "e2e-status-clean-" + t.Name()
	tag := "v1.0"

	// Push then pull
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"status-clean\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\npackages: []\n",
	)
	res := runCLI(t, srcDir, "push", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s", res.Stderr)
	}

	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", repoName+":"+tag, "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("pull failed: %s", res.Stderr)
	}

	// Check status
	res = runCLI(t, dstDir, "status")
	if res.ExitCode != 0 {
		t.Fatalf("status failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "clean") {
		t.Errorf("expected 'clean' status, got: %s", res.Stdout)
	}
}

func TestE2E_StatusModified(t *testing.T) {
	repoName := "e2e-status-mod-" + t.Name()
	tag := "v1.0"

	// Push then pull
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"status-mod\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\npackages: []\n",
	)
	res := runCLI(t, srcDir, "push", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s", res.Stderr)
	}

	dstDir := t.TempDir()
	res = runCLI(t, dstDir, "pull", repoName+":"+tag, "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("pull failed: %s", res.Stderr)
	}

	// Modify pixi.toml
	os.WriteFile(filepath.Join(dstDir, "pixi.toml"), []byte("[project]\nname = \"modified\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\nnumpy = \"*\"\n"), 0644)

	// Check status (should exit 1 for modified)
	res = runCLI(t, dstDir, "status")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit for modified status, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "modified") {
		t.Errorf("expected 'modified' status, got: %s", res.Stdout)
	}
}

func TestE2E_PushMultipleVersions(t *testing.T) {
	repoName := "e2e-multi-" + t.Name()

	// Push v1
	srcDir := t.TempDir()
	toml1 := "[project]\nname = \"multi-v1\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock1 := "version: 6\npackages:\n  - name: numpy\n    version: \"1.0\"\n"
	writePixiFiles(t, srcDir, toml1, lock1)

	res := runCLI(t, srcDir, "push", repoName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("push v1 failed: %s", res.Stderr)
	}

	// Push v2 with different content
	toml2 := "[project]\nname = \"multi-v2\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\nscipy = \"*\"\n"
	lock2 := "version: 6\npackages:\n  - name: scipy\n    version: \"1.12\"\n"
	writePixiFiles(t, srcDir, toml2, lock2)

	res = runCLI(t, srcDir, "push", repoName+":v2")
	if res.ExitCode != 0 {
		t.Fatalf("push v2 failed: %s", res.Stderr)
	}

	// Pull v1 and verify
	dir1 := t.TempDir()
	res = runCLI(t, dir1, "pull", repoName+":v1", "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("pull v1 failed: %s", res.Stderr)
	}
	got1, _ := os.ReadFile(filepath.Join(dir1, "pixi.toml"))
	if string(got1) != toml1 {
		t.Errorf("v1 pixi.toml mismatch:\ngot:  %s\nwant: %s", got1, toml1)
	}

	// Pull v2 and verify
	dir2 := t.TempDir()
	res = runCLI(t, dir2, "pull", repoName+":v2", "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("pull v2 failed: %s", res.Stderr)
	}
	got2, _ := os.ReadFile(filepath.Join(dir2, "pixi.toml"))
	if string(got2) != toml2 {
		t.Errorf("v2 pixi.toml mismatch:\ngot:  %s\nwant: %s", got2, toml2)
	}
}

func TestE2E_PushDryRun(t *testing.T) {
	repoName := "e2e-dryrun-" + t.Name()
	tag := "v1.0"

	// Push an initial version so dry-run has something to diff against
	srcDir := t.TempDir()
	toml := "[project]\nname = \"dryrun\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\n"
	writePixiFiles(t, srcDir, toml, lock)

	res := runCLI(t, srcDir, "push", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("initial push failed: %s", res.Stderr)
	}

	// Modify and do dry-run push
	modifiedToml := "[project]\nname = \"dryrun\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\nnumpy = \"*\"\n"
	os.WriteFile(filepath.Join(srcDir, "pixi.toml"), []byte(modifiedToml), 0644)

	res = runCLI(t, srcDir, "push", repoName+":v2", "--dry-run")
	if res.ExitCode != 0 {
		t.Fatalf("dry-run failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Would push") {
		t.Errorf("expected 'Would push' in dry-run output, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "without --dry-run") {
		t.Errorf("expected hint to run without --dry-run, got: %s", res.Stdout)
	}
}

func TestE2E_PublishToOCI(t *testing.T) {
	repoName := "e2e-publish-oci"
	tag := "v1.0"

	// Push a version first
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"publish-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\npackages: []\n",
	)
	res := runCLI(t, srcDir, "push", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s", res.Stderr)
	}

	// Register the OCI registry on the server
	// Keep http:// prefix so the server knows to use PlainHTTP for this registry
	ociURL := e2eEnv.ociRegistry.URL // e.g., "http://127.0.0.1:PORT"

	res = runCLI(t, srcDir, "registry", "add", "test-oci", ociURL, "--default")
	if res.ExitCode != 0 {
		t.Fatalf("registry add failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Publish to OCI
	res = runCLI(t, srcDir, "publish", repoName+":"+tag)
	if res.ExitCode != 0 {
		t.Fatalf("publish failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Published") {
		t.Errorf("expected 'Published' in output, got: %s", res.Stdout)
	}

	// Verify artifact exists in the OCI registry by checking the tag
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", ociURL, repoName, tag)
	req, _ := http.NewRequest("GET", manifestURL, nil)
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to check OCI manifest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("OCI manifest not found (status %d), expected 200", resp.StatusCode)
	}
}

func TestE2E_RepoList(t *testing.T) {
	repoName := "e2e-repolist-" + t.Name()

	// Create a repo
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"repolist\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)
	res := runCLI(t, dir, "push", repoName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s", res.Stderr)
	}

	// List repos
	res = runCLI(t, dir, "repo", "list")
	if res.ExitCode != 0 {
		t.Fatalf("repo list failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, repoName) {
		t.Errorf("expected repo %q in list output, got: %s", repoName, res.Stdout)
	}
}

func TestE2E_RepoDelete(t *testing.T) {
	repoName := "e2e-repodelete-" + t.Name()

	// Create a repo
	dir := t.TempDir()
	writePixiFiles(t, dir,
		"[project]\nname = \"repodelete\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)
	res := runCLI(t, dir, "push", repoName+":v1")
	if res.ExitCode != 0 {
		t.Fatalf("push failed: %s", res.Stderr)
	}

	// Delete it
	res = runCLI(t, dir, "repo", "delete", repoName)
	if res.ExitCode != 0 {
		t.Fatalf("repo delete failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", res.Stdout)
	}

	// Verify it's gone
	res = runCLI(t, dir, "pull", repoName+":v1", "--yes")
	if res.ExitCode == 0 {
		t.Error("expected pull to fail after delete")
	}
}
