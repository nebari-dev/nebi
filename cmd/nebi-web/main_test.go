package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/pixi"
)

// These tests run nebi-web for real: the test binary runs again with
// mainEnv set and calls main with the arguments in mainArgsEnv.
const (
	mainEnv     = "NEBI_TEST_WEB_MAIN"
	mainArgsEnv = "NEBI_TEST_WEB_ARGS"
	waitTimeout = 30 * time.Second
)

func TestMain(m *testing.M) {
	if os.Getenv(mainEnv) != "" {
		var args []string
		if err := json.Unmarshal([]byte(os.Getenv(mainArgsEnv)), &args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		os.Args = append([]string{"nebi-web"}, args...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type webProcess struct {
	t     *testing.T
	cmd   *exec.Cmd
	lines chan string
	done  chan struct{}
	err   syncBuffer
	// out holds the stdout lines signInURL has read.
	out []string
}

// runWeb starts nebi-web with args against dataDir, with a stub pixi that
// only answers --version.
func runWeb(t *testing.T, dataDir string, args ...string) *webProcess {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub pixi is a shell script")
	}
	stubDir := t.TempDir()
	stub := filepath.Join(stubDir, "pixi")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho pixi "+pixi.MinimumVersion+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(args)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		mainEnv+"=1",
		mainArgsEnv+"="+string(encoded),
		"NEBI_DATA_DIR="+dataDir,
		"NEBI_PROJECTS_DIR="+filepath.Join(t.TempDir(), "projects"),
		"NEBI_PIXI_PATH="+stub,
	)
	p := &webProcess{t: t, cmd: cmd, lines: make(chan string, 64), done: make(chan struct{})}
	cmd.Stderr = &p.err
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			p.lines <- s.Text()
		}
		close(p.lines)
		_ = cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		// Read what the test left unread, so the reader reaches Wait and
		// cleanup cannot hang on a nebi-web that printed a lot.
		go func() {
			for range p.lines {
			}
		}()
		select {
		case <-p.done:
		case <-time.After(waitTimeout):
			t.Errorf("nebi-web %d did not exit after it was killed", cmd.Process.Pid)
		}
	})
	return p
}

// signInURL reads stdout until the URL nebi-web prints.
func (p *webProcess) signInURL() string {
	p.t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				<-p.done
				p.t.Fatalf("nebi-web exited before printing a URL: %s", p.err.String())
			}
			p.out = append(p.out, line)
			if u := strings.TrimSpace(line); strings.HasPrefix(u, "http://") {
				return u
			}
		case <-deadline:
			p.t.Fatal("nebi-web printed no URL")
		}
	}
}

// wait waits for nebi-web to exit and returns its exit code. Output the
// test has not read is dropped, so it cannot keep nebi-web from exiting.
func (p *webProcess) wait() int {
	p.t.Helper()
	go func() {
		for range p.lines {
		}
	}()
	select {
	case <-p.done:
	case <-time.After(waitTimeout):
		p.t.Fatal("nebi-web did not exit")
	}
	return p.cmd.ProcessState.ExitCode()
}

// syncBuffer collects a process's stderr, which a test may read while the
// process still writes it.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func requireLockFree(t *testing.T, dir string) {
	t.Helper()
	o, err := jobs.TryAcquireOwner(dir)
	if err != nil {
		t.Fatalf("the owner lock is not free: %v", err)
	}
	_ = o.Release()
}

// nebi-web prints the sign-in URL, serves on loopback, and on SIGINT or
// SIGTERM shuts down, removing owner.json and releasing the lock. A second
// nebi-web prints the running one's URL and exits.
func TestServesAndStopsOnSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := t.TempDir()
			port := freePort(t)
			web := runWeb(t, dir, "--port", strconv.Itoa(port))
			url := web.signInURL()
			info, err := jobs.ReadOwner(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", port, info.Token)
			if url != want || info.Kind != jobs.OwnerWeb || info.Address != fmt.Sprintf("127.0.0.1:%d", port) {
				t.Fatalf("printed %s with owner.json %v, want %s", url, info, want)
			}
			if _, err := os.Stat(filepath.Join(dir, "nebi-client.db")); err != nil {
				t.Fatalf("nebi-client.db: %v", err)
			}

			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/version", port))
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /version = %d", resp.StatusCode)
			}

			// A second nebi-web on the same data directory prints the
			// running one's URL on stdout and exits 0, without starting.
			second := runWeb(t, dir)
			if got := second.signInURL(); got != url {
				t.Fatalf("second nebi-web printed %s, want the running one's %s", got, url)
			}
			if out := strings.Join(second.out, "\n"); !strings.Contains(out, "Nebi is already running (nebi-web, process "+
				strconv.Itoa(web.cmd.Process.Pid)+")") || strings.Contains(out, "Press Ctrl+C") {
				t.Fatalf("second nebi-web printed:\n%s\nwant that nebi-web is already running", out)
			}
			if code := second.wait(); code != 0 {
				t.Fatalf("second nebi-web exited with %d: %s", code, second.err.String())
			}
			if strings.Contains(second.err.String(), url) {
				t.Fatalf("second nebi-web wrote the URL to stderr: %s", second.err.String())
			}

			if err := web.cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			if code := web.wait(); code != 0 {
				t.Fatalf("nebi-web exited with %d after %s: %s", code, sig, web.err.String())
			}
			if _, err := os.Stat(filepath.Join(dir, jobs.OwnerInfoFile)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("owner.json after %s: %v", sig, err)
			}
			requireLockFree(t, dir)
			if strings.Contains(web.err.String(), info.Token) {
				t.Fatalf("the token was written to stderr: %s", web.err.String())
			}
		})
	}
}

func TestHostFlagRemoved(t *testing.T) {
	dir := t.TempDir()
	web := runWeb(t, dir, "--host", "0.0.0.0")
	if code := web.wait(); code != 1 || !strings.Contains(web.err.String(), "--host was removed") {
		t.Fatalf("nebi-web --host exited with %d: %s", code, web.err.String())
	}
	if _, err := os.Stat(filepath.Join(dir, jobs.OwnerLockFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nebi-web --host touched the data directory: %v", err)
	}
}

func TestPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	web := runWeb(t, dir, "--port", strconv.Itoa(port))
	code := web.wait()
	msg := web.err.String()
	if code != 1 || !strings.Contains(msg, "already in use") || !strings.Contains(msg, strconv.Itoa(port)) || !strings.Contains(msg, "--port") {
		t.Fatalf("nebi-web on a taken port exited with %d: %s", code, msg)
	}
	requireLockFree(t, dir)
}

// A port out of range is refused before nebi-web touches the data
// directory.
func TestPortOutOfRange(t *testing.T) {
	for _, port := range []string{"0", "-1", "65536", "70000"} {
		t.Run(port, func(t *testing.T) {
			dir := t.TempDir()
			web := runWeb(t, dir, "--port", port)
			if code := web.wait(); code != 1 || !strings.Contains(web.err.String(), "--port "+port+" is not a port number") {
				t.Fatalf("nebi-web --port %s exited with %d: %s", port, code, web.err.String())
			}
			if _, err := os.Stat(filepath.Join(dir, jobs.OwnerLockFile)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("nebi-web --port %s touched the data directory: %v", port, err)
			}
		})
	}
}

// Ctrl+C while nebi-web waits for a CLI command is a normal stop: no error,
// exit 0.
func TestInterruptWhileWaiting(t *testing.T) {
	dir := t.TempDir()
	cli, err := jobs.TryAcquireOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Release()
	if err := cli.Publish(jobs.OwnerInfo{Kind: jobs.OwnerCLI, NebiVersion: "1.2.3", ClientAPIVersion: "1.0"}); err != nil {
		t.Fatal(err)
	}
	web := runWeb(t, dir, "--port", strconv.Itoa(freePort(t)))
	deadline := time.Now().Add(waitTimeout)
	for !strings.Contains(web.err.String(), "Waiting for process") {
		if time.Now().After(deadline) {
			t.Fatalf("nebi-web did not wait for the CLI command: %s", web.err.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := web.cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if code := web.wait(); code != 0 || strings.Contains(web.err.String(), "Error") {
		t.Fatalf("nebi-web interrupted while waiting exited with %d: %s", code, web.err.String())
	}
}

// The token comes only from the owner, never from the command line.
func TestNoTokenFlag(t *testing.T) {
	for _, name := range []string{"token", "api-token", "session-token"} {
		if rootCmd.Flags().Lookup(name) != nil {
			t.Errorf("nebi-web has a --%s flag", name)
		}
	}
}
