//go:build unix

package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/pixi"
)

// These tests run a stub pixi, a shell script, so they can check the
// arguments, environment and process handling exactly. pixi_test.go runs
// the real pixi.

const waitTimeout = 15 * time.Second

// stub is a fake pixi binary. Every call except --version records its
// working directory, arguments and two environment variables in calls,
// writes a line to stdout and to stderr, and then runs body.
type stub struct {
	path  string
	calls string
}

func newStub(t *testing.T, version, body string) *stub {
	t.Helper()
	dir := t.TempDir()
	s := &stub{path: filepath.Join(dir, "pixi"), calls: filepath.Join(dir, "calls")}
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then echo "pixi %s"; exit 0; fi
printf '%%s|%%s|%%s|%%s\n' "$PWD" "$*" "${PIXI_LOCKED-unset}" "${PIXI_COLOR-unset}" >> %q
echo "stdout from pixi $1"
echo "stderr from pixi $1" >&2
%s
`, version, s.calls, body)
	if err := os.WriteFile(s.path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return s
}

// recorded returns one line per call: dir|args|PIXI_LOCKED|PIXI_COLOR.
func (s *stub) recorded(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(s.calls)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func newRunner(t *testing.T, path string) *Runner {
	t.Helper()
	r, err := New(context.Background(), Options{PixiPath: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

// project returns a directory with a pixi.toml and, when lock is true, a
// pixi.lock. Its path has no symlinks, so it matches what pixi sees as
// its working directory.
func project(t *testing.T, lock bool) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[workspace]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if lock {
		if err := os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte("version: 6\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCommands(t *testing.T) {
	s := newStub(t, pixi.MinimumVersion, "exit 0")
	r := newRunner(t, s.path)
	dir := project(t, true)
	t.Setenv("PIXI_LOCKED", "true")
	t.Setenv("PIXI_COLOR", "always")
	ctx := context.Background()

	var out bytes.Buffer
	if err := r.Lock(ctx, dir, &out); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := r.Install(ctx, dir, &out); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := r.CheckLock(ctx, dir, &out); err != nil {
		t.Fatalf("CheckLock: %v", err)
	}

	manifest := filepath.Join(dir, "pixi.toml")
	want := []string{
		dir + "|lock --manifest-path " + manifest + "|unset|never",
		dir + "|install --frozen --manifest-path " + manifest + "|unset|never",
		dir + "|lock --check --manifest-path " + manifest + "|unset|never",
	}
	got := s.recorded(t)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("pixi calls:\n got %q\nwant %q", got, want)
	}
	for _, sub := range []string{"lock", "install"} {
		for _, stream := range []string{"stdout", "stderr"} {
			if line := stream + " from pixi " + sub; !strings.Contains(out.String(), line) {
				t.Errorf("out is missing %q: %q", line, out.String())
			}
		}
	}
}

func TestFailedCommandCarriesPixiError(t *testing.T) {
	s := newStub(t, pixi.MinimumVersion, `cat >&2 <<'EOF'
Error:   × failed to solve requirements of environment 'default' for platform 'osx-
  │ arm64'
  ╰─▶ Cannot solve the request because of: No candidates were found for nope *.
EOF
exit 1`)
	r := newRunner(t, s.path)
	var out bytes.Buffer
	err := r.Lock(context.Background(), project(t, false), &out)
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("want a *CommandError, got %T %v", err, err)
	}
	want := "pixi lock: failed to solve requirements of environment 'default' for platform 'osx-arm64': Cannot solve the request because of: No candidates were found for nope *. (exit status 1)"
	if err.Error() != want {
		t.Fatalf("got  %q\nwant %q", err, want)
	}
	if !strings.Contains(out.String(), "No candidates were found for nope") {
		t.Fatalf("pixi's error output must reach out too, got %q", out.String())
	}
}

func TestCheckLock(t *testing.T) {
	stale := newStub(t, pixi.MinimumVersion, `echo "Error:   × lock file not up-to-date with the workspace" >&2; exit 1`)
	broken := newStub(t, pixi.MinimumVersion, `echo "Error:   × Request failed after 3 retries" >&2; exit 1`)
	ctx := context.Background()

	err := newRunner(t, stale.path).CheckLock(ctx, project(t, true), io.Discard)
	var staleErr *StaleLockError
	if !errors.As(err, &staleErr) || staleErr.Detail != staleLockMessage {
		t.Fatalf("stale lock: want a *StaleLockError with pixi's message, got %T %v", err, err)
	}

	err = newRunner(t, broken.path).CheckLock(ctx, project(t, true), io.Discard)
	var cmdErr *CommandError
	if errors.As(err, &staleErr) || !errors.As(err, &cmdErr) || !strings.Contains(err.Error(), "Request failed after 3 retries") {
		t.Fatalf("failed check: want a *CommandError that is not stale, got %T %v", err, err)
	}

	// A missing pixi.lock is stale without running pixi; a missing
	// pixi.toml is an ordinary error.
	err = newRunner(t, broken.path).CheckLock(ctx, project(t, false), io.Discard)
	if !errors.As(err, &staleErr) || !strings.Contains(err.Error(), "pixi.lock is missing") {
		t.Fatalf("missing pixi.lock: want a *StaleLockError, got %T %v", err, err)
	}
	err = newRunner(t, broken.path).CheckLock(ctx, t.TempDir(), io.Discard)
	if err == nil || errors.As(err, &staleErr) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing pixi.toml: want a not-exist error that is not stale, got %v", err)
	}
	if calls := broken.recorded(t); len(calls) != 1 {
		t.Fatalf("pixi must run only for the check with both files, got %q", calls)
	}
}

func TestInstallWithoutLockDoesNotRunPixi(t *testing.T) {
	s := newStub(t, pixi.MinimumVersion, "exit 0")
	err := newRunner(t, s.path).Install(context.Background(), project(t, false), io.Discard)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "pixi.lock") {
		t.Fatalf("want an error naming the missing pixi.lock, got %v", err)
	}
	if calls := s.recorded(t); len(calls) != 0 {
		t.Fatalf("pixi must not run, got %q", calls)
	}
}

func TestNewResolvesBinaryOnce(t *testing.T) {
	s := newStub(t, "0.84.2", "exit 0")
	t.Setenv("PATH", filepath.Dir(s.path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := newRunner(t, "pixi")
	if r.Path() != s.path {
		t.Fatalf("Path = %q, want %q", r.Path(), s.path)
	}
	if r.Version() != "0.84.2" {
		t.Fatalf("Version = %q", r.Version())
	}

	// Another pixi earlier in PATH later does not change the binary.
	other := newStub(t, "0.84.2", "exit 0")
	t.Setenv("PATH", filepath.Dir(other.path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := r.Lock(context.Background(), project(t, false), io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(s.recorded(t)) != 1 || len(other.recorded(t)) != 0 {
		t.Fatalf("Lock must run the binary New resolved: resolved %q, other %q", s.recorded(t), other.recorded(t))
	}
}

func TestNewRefusesOldOrBrokenPixi(t *testing.T) {
	ctx := context.Background()
	old := newStub(t, "0.58.0", "exit 0")
	_, err := New(ctx, Options{PixiPath: old.path})
	var tooOld *pixi.VersionTooOldError
	if !errors.As(err, &tooOld) || tooOld.Found != "0.58.0" || tooOld.Minimum != pixi.MinimumVersion {
		t.Fatalf("pixi 0.58.0: want a *pixi.VersionTooOldError, got %v", err)
	}
	for _, msg := range []string{"0.58.0", pixi.MinimumVersion, "upgrade"} {
		if !strings.Contains(err.Error(), msg) {
			t.Errorf("the error must mention %q: %v", msg, err)
		}
	}

	noVersion := filepath.Join(t.TempDir(), "pixi")
	if err := os.WriteFile(noVersion, []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(ctx, Options{PixiPath: noVersion}); err == nil {
		t.Fatal("a binary that reports no version must be refused")
	}

	// A configured path that does not exist fails; nothing is installed in
	// its place (#630).
	t.Setenv("HOME", t.TempDir())
	missing := filepath.Join(t.TempDir(), "no-pixi")
	if _, err := New(ctx, Options{PixiPath: missing}); err == nil {
		t.Fatal("a missing configured pixi must be refused")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".local", "bin", "pixi")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("nothing may be installed for a configured path, stat: %v", err)
	}
}

// TestCancelStopsPixiAndChildren starts a pixi that starts a child of its
// own, as pixi does, and cancels it: both must stop and Lock must return
// promptly with the context's error.
func TestCancelStopsPixiAndChildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pids")
	s := newStub(t, pixi.MinimumVersion, fmt.Sprintf(`sleep 300 & echo "$$ $!" > %[1]q.tmp && mv %[1]q.tmp %[1]q; wait`, pidFile))
	r := newRunner(t, s.path)

	dir := project(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	var out bytes.Buffer
	go func() { done <- r.Lock(ctx, dir, &out) }()

	pids := readPIDs(t, pidFile)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want the context's error, got %v", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Lock did not return after cancel")
	}
	for _, pid := range pids {
		waitGone(t, pid)
	}
}

func TestCancelledContextDoesNotStartPixi(t *testing.T) {
	s := newStub(t, pixi.MinimumVersion, "exit 0")
	r := newRunner(t, s.path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Lock(ctx, project(t, true), io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if calls := s.recorded(t); len(calls) != 0 {
		t.Fatalf("pixi must not start, got %q", calls)
	}
}

// readPIDs waits for the stub to record its PID and its child's.
func readPIDs(t *testing.T, path string) []int {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			var pids []int
			for _, f := range strings.Fields(string(data)) {
				pid, err := strconv.Atoi(f)
				if err != nil {
					t.Fatalf("bad PID file %q", data)
				}
				pids = append(pids, pid)
			}
			return pids
		}
		if time.Now().After(deadline) {
			t.Fatal("the stub did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitGone waits until pid no longer exists. The stub's shell is reaped by
// os/exec; its sleep child is reparented and reaped by init.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still running after cancel", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
