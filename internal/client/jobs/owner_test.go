package jobs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Owner lock tests that need a real second process run this test binary
// again with ownerHelperEnv set; TestOwnerHelperProcess then plays the other
// process instead of running as a test.
const (
	ownerHelperEnv    = "NEBI_TEST_OWNER_HELPER"
	ownerHelperDirEnv = "NEBI_TEST_OWNER_DIR"

	// helperTimeout bounds every wait on a helper process. It is far above
	// what any step needs, so it only trips when a step is really stuck.
	helperTimeout = 20 * time.Second
)

var (
	// thisHost is the host name Publish records for every owner in these
	// tests, all of which run on this machine.
	thisHost = func() string {
		h, err := os.Hostname()
		if err != nil {
			panic(err)
		}
		return h
	}()

	helperNoAPI = OwnerInfo{Kind: OwnerCLI, Hostname: thisHost, NebiVersion: "1.2.3", ClientAPIVersion: "v1"}
	helperAPI   = OwnerInfo{Kind: OwnerWeb, Hostname: thisHost, NebiVersion: "1.2.3", ClientAPIVersion: "v1",
		Address: "127.0.0.1:43210", Token: "helper-token"}
)

// TestOwnerHelperProcess is the second process of the owner lock tests. It
// is not a test: without ownerHelperEnv it returns at once.
//
// Modes:
//   - try: TryAcquireOwner, print "acquired" or "held". The owner then obeys
//     stdin: "publish" publishes helperNoAPI, "publish-api" publishes
//     helperAPI, and EOF releases the lock and prints "released".
//   - wait: like try, but when the lock is held it prints "waiting" and
//     takes the lock with AcquireOwner.
//   - spawn: take the lock, publish, start a long-lived child process, print
//     "child <pid>", then block until killed.
//   - sleep: block for a minute; the parent test kills it.
func TestOwnerHelperProcess(t *testing.T) {
	mode := os.Getenv(ownerHelperEnv)
	if mode == "" {
		return
	}
	dir := os.Getenv(ownerHelperDirEnv)
	out := func(format string, args ...any) { fmt.Fprintf(os.Stdout, format+"\n", args...) }
	fail := func(err error) {
		out("error: %v", err)
		os.Exit(2)
	}

	switch mode {
	case "sleep":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "try", "wait", "spawn":
	default:
		fail(fmt.Errorf("unknown helper mode %q", mode))
	}

	var (
		o   *Owner
		err error
	)
	o, err = TryAcquireOwner(dir)
	if mode == "wait" && errors.Is(err, ErrOwnerLockHeld) {
		out("waiting")
		o, err = AcquireOwner(context.Background(), dir)
	}
	if errors.Is(err, ErrOwnerLockHeld) {
		out("held")
		os.Exit(0)
	}
	if err != nil {
		fail(err)
	}

	if mode == "spawn" {
		if err := o.Publish(helperNoAPI); err != nil {
			fail(err)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestOwnerHelperProcess$")
		child.Env = append(os.Environ(), ownerHelperEnv+"=sleep")
		if err := child.Start(); err != nil {
			fail(err)
		}
		out("child %d", child.Process.Pid)
		time.Sleep(time.Minute) // until killed
		os.Exit(3)
	}

	out("acquired")
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		switch in.Text() {
		case "publish":
			err = o.Publish(helperNoAPI)
		case "publish-api":
			err = o.Publish(helperAPI)
		default:
			err = fmt.Errorf("unknown helper command %q", in.Text())
		}
		if err != nil {
			fail(err)
		}
		out("published")
	}
	if err := o.Release(); err != nil {
		fail(err)
	}
	out("released")
	os.Exit(0)
}

type ownerHelper struct {
	t     *testing.T
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	done  chan struct{}
}

// startOwnerHelper starts TestOwnerHelperProcess in mode against dir. The
// helper is killed when the test ends if it is still running.
func startOwnerHelper(t *testing.T, mode, dir string) *ownerHelper {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnerHelperProcess$")
	cmd.Env = append(os.Environ(), ownerHelperEnv+"="+mode, ownerHelperDirEnv+"="+dir)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// The buffer holds more lines than any helper prints, so the reader
	// never blocks and reaches EOF even when a test stops reading early.
	h := &ownerHelper{t: t, cmd: cmd, stdin: stdin, lines: make(chan string, 64), done: make(chan struct{})}
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			h.lines <- s.Text()
		}
		close(h.lines)
		// Wait only after stdout is drained, as exec.Cmd requires.
		_ = cmd.Wait()
		close(h.done)
	}()
	t.Cleanup(func() {
		_ = h.cmd.Process.Kill()
		<-h.done
	})
	return h
}

// next returns the helper's next output line.
func (h *ownerHelper) next() string {
	h.t.Helper()
	select {
	case line, ok := <-h.lines:
		if !ok {
			h.t.Fatalf("helper %d exited before printing a line", h.cmd.Process.Pid)
		}
		if strings.HasPrefix(line, "error:") {
			h.t.Fatalf("helper %d: %s", h.cmd.Process.Pid, line)
		}
		return line
	case <-time.After(helperTimeout):
		h.t.Fatalf("helper %d printed nothing for %v", h.cmd.Process.Pid, helperTimeout)
		return ""
	}
}

func (h *ownerHelper) expect(want string) {
	h.t.Helper()
	if got := h.next(); got != want {
		h.t.Fatalf("helper %d printed %q, want %q", h.cmd.Process.Pid, got, want)
	}
}

// kill kills the helper, which must still be running, and waits for it.
func (h *ownerHelper) kill() {
	h.t.Helper()
	select {
	case <-h.done:
		h.t.Fatalf("helper %d exited before it was killed: %v", h.cmd.Process.Pid, h.cmd.ProcessState)
	default:
	}
	if err := h.cmd.Process.Kill(); err != nil {
		h.t.Fatalf("killing helper %d: %v", h.cmd.Process.Pid, err)
	}
	select {
	case <-h.done:
	case <-time.After(helperTimeout):
		h.t.Fatalf("helper %d did not exit after it was killed", h.cmd.Process.Pid)
	}
}

func (h *ownerHelper) send(cmd string) {
	h.t.Helper()
	if _, err := io.WriteString(h.stdin, cmd+"\n"); err != nil {
		h.t.Fatalf("writing to helper: %v", err)
	}
}

func ownerTestDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func mustAcquire(t *testing.T, dir string) *Owner {
	t.Helper()
	o, err := TryAcquireOwner(dir)
	if err != nil {
		t.Fatalf("TryAcquireOwner: %v", err)
	}
	t.Cleanup(func() { _ = o.Release() })
	return o
}

// acquireAfterOwnerDied takes the lock that a killed owner held, with no
// cleanup step in between. On Unix the kernel closes a process's files before
// the process can be waited for, so the very next try must succeed. Windows
// frees a dead process's file locks a little later and gives no point at
// which they are known to be free, so there the test waits for the lock.
func acquireAfterOwnerDied(t *testing.T, dir string) *Owner {
	t.Helper()
	if runtime.GOOS != "windows" {
		return mustAcquire(t, dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	o, err := AcquireOwner(ctx, dir)
	if err != nil {
		t.Fatalf("AcquireOwner after the owner was killed: %v", err)
	}
	t.Cleanup(func() { _ = o.Release() })
	return o
}

func requireHeld(t *testing.T, dir string) {
	t.Helper()
	o, err := TryAcquireOwner(dir)
	if err == nil {
		_ = o.Release()
		t.Fatal("TryAcquireOwner took a lock another owner holds")
	}
	if !errors.Is(err, ErrOwnerLockHeld) {
		t.Fatalf("TryAcquireOwner: got %v, want ErrOwnerLockHeld", err)
	}
}

func TestOwnerLockOneOwnerAcrossProcesses(t *testing.T) {
	dir := ownerTestDir(t)

	const n = 4
	helpers := make([]*ownerHelper, n)
	for i := range helpers {
		helpers[i] = startOwnerHelper(t, "try", dir)
	}
	var winner *ownerHelper
	for _, h := range helpers {
		switch got := h.next(); got {
		case "acquired":
			if winner != nil {
				t.Fatalf("processes %d and %d both took the lock", winner.cmd.Process.Pid, h.cmd.Process.Pid)
			}
			winner = h
		case "held":
		default:
			t.Fatalf("helper printed %q", got)
		}
	}
	if winner == nil {
		t.Fatal("no process took the lock")
	}
	requireHeld(t, dir)

	// Holding the lock without having published: no owner info yet, which
	// readers must treat as "wait", not "no owner".
	if _, err := ReadOwner(dir); !errors.Is(err, ErrNoOwnerInfo) {
		t.Fatalf("ReadOwner before publish: got %v, want ErrNoOwnerInfo", err)
	}

	winner.send("publish")
	winner.expect("published")
	info, err := ReadOwner(dir)
	if err != nil {
		t.Fatalf("ReadOwner: %v", err)
	}
	want := helperNoAPI
	want.PID = winner.cmd.Process.Pid
	if info != want {
		t.Fatalf("ReadOwner = %+v, want %+v", info, want)
	}

	winner.send("publish-api")
	winner.expect("published")
	if info, err = ReadOwner(dir); err != nil {
		t.Fatalf("ReadOwner: %v", err)
	}
	want = helperAPI
	want.PID = winner.cmd.Process.Pid
	if info != want {
		t.Fatalf("ReadOwner after publishing the API = %+v, want %+v", info, want)
	}

	_ = winner.stdin.Close()
	winner.expect("released")
	if _, err := ReadOwner(dir); !errors.Is(err, ErrNoOwnerInfo) {
		t.Fatalf("ReadOwner after release: got %v, want ErrNoOwnerInfo", err)
	}
	mustAcquire(t, dir)
}

func TestOwnerLockFreedWhenOwnerIsKilled(t *testing.T) {
	dir := ownerTestDir(t)

	h := startOwnerHelper(t, "try", dir)
	h.expect("acquired")
	h.send("publish-api")
	h.expect("published")
	requireHeld(t, dir)

	h.kill()

	// The dead owner's file is still there; nothing has cleaned up.
	if _, err := os.Stat(filepath.Join(dir, OwnerInfoFile)); err != nil {
		t.Fatalf("owner.json of the killed owner: %v", err)
	}
	// The lock is taken with no cleanup step in between, and the new owner
	// has removed the dead owner's details.
	o := acquireAfterOwnerDied(t, dir)
	if _, err := ReadOwner(dir); !errors.Is(err, ErrNoOwnerInfo) {
		t.Fatalf("ReadOwner after a new owner took the lock: got %v, want ErrNoOwnerInfo", err)
	}
	if err := o.Publish(helperNoAPI); err != nil {
		t.Fatal(err)
	}
	info, err := ReadOwner(dir)
	if err != nil || info.PID != os.Getpid() {
		t.Fatalf("ReadOwner = %+v, %v; want this process", info, err)
	}
}

// An owner runs jobs as child processes (pixi). If the owner dies while one
// runs, the child must not keep the lock alive.
func TestOwnerLockNotHeldByChildOfKilledOwner(t *testing.T) {
	dir := ownerTestDir(t)

	h := startOwnerHelper(t, "spawn", dir)
	line := h.next()
	pidText, ok := strings.CutPrefix(line, "child ")
	if !ok {
		t.Fatalf("helper printed %q, want child <pid>", line)
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatal(err)
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Kill()
		_, _ = child.Wait() // not our child on Unix; the error is harmless
	})
	requireHeld(t, dir)

	h.kill()

	acquireAfterOwnerDied(t, dir)
	// The lock was free while the child still ran.
	if err := processRunning(pid); err != nil {
		t.Fatalf("child %d of the killed owner is not running, so the test proved nothing: %v", pid, err)
	}
}

func TestAcquireOwnerWaitsForRelease(t *testing.T) {
	dir := ownerTestDir(t)

	h := startOwnerHelper(t, "try", dir)
	h.expect("acquired")

	type result struct {
		o   *Owner
		err error
	}
	got := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	go func() {
		o, err := AcquireOwner(ctx, dir)
		got <- result{o, err}
	}()

	select {
	case r := <-got:
		if r.o != nil {
			_ = r.o.Release()
		}
		t.Fatalf("AcquireOwner returned while another process held the lock: %v", r.err)
	case <-time.After(10 * lockPollInterval):
	}

	_ = h.stdin.Close()
	h.expect("released")

	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("AcquireOwner: %v", r.err)
		}
		t.Cleanup(func() { _ = r.o.Release() })
	case <-time.After(helperTimeout):
		t.Fatal("AcquireOwner did not return after the lock was released")
	}
	requireHeld(t, dir)
}

func TestAcquireOwnerWaitingProcessTakesOver(t *testing.T) {
	dir := ownerTestDir(t)
	o := mustAcquire(t, dir)

	h := startOwnerHelper(t, "wait", dir)
	// The helper found the lock held and is now waiting in AcquireOwner.
	h.expect("waiting")
	// Give it a few retries; it must not take the lock meanwhile.
	time.Sleep(4 * lockPollInterval)
	select {
	case line := <-h.lines:
		t.Fatalf("waiting helper printed %q while this process held the lock", line)
	default:
	}

	if err := o.Release(); err != nil {
		t.Fatal(err)
	}
	h.expect("acquired")
	requireHeld(t, dir)
}

func TestAcquireOwnerStopsWhenContextEnds(t *testing.T) {
	dir := ownerTestDir(t)
	mustAcquire(t, dir)

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*lockPollInterval)
		defer cancel()
		err := acquireWithin(t, ctx, dir)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want an error wrapping context.DeadlineExceeded", err)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(4*lockPollInterval, cancel)
		err := acquireWithin(t, ctx, dir)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want an error wrapping context.Canceled", err)
		}
	})

	t.Run("already done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := acquireWithin(t, ctx, dir)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want an error wrapping context.Canceled", err)
		}
	})

	// Giving up did not take or leak the lock.
	requireHeld(t, dir)
}

// acquireWithin runs AcquireOwner and fails the test if it does not return
// in time or returns an owner.
func acquireWithin(t *testing.T, ctx context.Context, dir string) error {
	t.Helper()
	got := make(chan error, 1)
	go func() {
		o, err := AcquireOwner(ctx, dir)
		if o != nil {
			_ = o.Release()
			err = errors.New("AcquireOwner took a lock another owner holds")
		}
		got <- err
	}()
	select {
	case err := <-got:
		return err
	case <-time.After(helperTimeout):
		t.Fatal("AcquireOwner did not return after its context ended")
		return nil
	}
}

// A relative dir is resolved once, so owner.json stays put if the working
// directory changes later. An empty dir is an error, not "wait".
func TestOwnerDirResolution(t *testing.T) {
	base := ownerTestDir(t)
	t.Chdir(base)
	o := mustAcquire(t, "data")
	want := filepath.Join(base, "data")
	if got := o.Dir(); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}

	t.Chdir(ownerTestDir(t))
	if err := o.Publish(helperNoAPI); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOwner(want); err != nil {
		t.Fatalf("ReadOwner after a change of working directory: %v", err)
	}
	if err := o.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(want, OwnerInfoFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner.json after release: %v, want it removed", err)
	}

	if _, err := TryAcquireOwner(""); err == nil || errors.Is(err, ErrOwnerLockHeld) {
		t.Fatalf("TryAcquireOwner(\"\"): got %v, want an empty directory error", err)
	}
	if _, err := ReadOwner(""); err == nil || errors.Is(err, ErrNoOwnerInfo) {
		t.Fatalf("ReadOwner(\"\"): got %v, want an empty directory error", err)
	}
}

// AcquireOwner resolves a relative dir once, before waiting. Resolved on
// every retry, a change of working directory during the wait would make it
// lock a different directory while the first is still held.
func TestAcquireOwnerResolvesDirOnce(t *testing.T) {
	base := ownerTestDir(t)
	other := ownerTestDir(t)
	want := filepath.Join(base, "data")
	held := mustAcquire(t, want)

	t.Chdir(base)
	type result struct {
		o   *Owner
		err error
	}
	got := make(chan result, 1)
	started := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	go func() {
		close(started)
		o, err := AcquireOwner(ctx, "data")
		got <- result{o, err}
	}()
	<-started
	// Let it resolve dir and retry a few times before moving.
	time.Sleep(4 * lockPollInterval)
	if err := os.Chdir(other); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-got:
		if r.o != nil {
			_ = r.o.Release()
			t.Fatalf("AcquireOwner took the lock in %s while %s was held", r.o.Dir(), want)
		}
		t.Fatalf("AcquireOwner returned while the lock was held: %v", r.err)
	case <-time.After(10 * lockPollInterval):
	}
	if _, err := os.Stat(filepath.Join(other, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AcquireOwner created %s: %v", filepath.Join(other, "data"), err)
	}

	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("AcquireOwner: %v", r.err)
		}
		t.Cleanup(func() { _ = r.o.Release() })
		if r.o.Dir() != want {
			t.Fatalf("AcquireOwner took the lock in %s, want %s", r.o.Dir(), want)
		}
	case <-time.After(helperTimeout):
		t.Fatal("AcquireOwner did not return after the lock was released")
	}
}

// Release removes owner.json while it still holds the lock. Removed after
// unlocking, it could be the next owner's file.
func TestReleaseRemovesOwnerInfoBeforeUnlock(t *testing.T) {
	dir := ownerTestDir(t)
	o := mustAcquire(t, dir)
	if err := o.Publish(helperAPI); err != nil {
		t.Fatal(err)
	}

	var (
		called  bool
		statErr error
		lockErr error
	)
	testHookBeforeUnlock = func(dir string) {
		if called {
			return
		}
		called = true
		_, statErr = os.Stat(filepath.Join(dir, OwnerInfoFile))
		next, err := TryAcquireOwner(dir)
		lockErr = err
		if err == nil {
			_ = next.Release()
		}
	}
	t.Cleanup(func() { testHookBeforeUnlock = nil })

	if err := o.Release(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("Release did not run the hook between its steps")
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("owner.json before unlock: %v, want it removed", statErr)
	}
	if !errors.Is(lockErr, ErrOwnerLockHeld) {
		t.Fatalf("TryAcquireOwner after owner.json was removed: got %v, want ErrOwnerLockHeld", lockErr)
	}
}

func TestOwnerLockIsPerHandleInOneProcess(t *testing.T) {
	dir := filepath.Join(ownerTestDir(t), "data", "nested")
	o := mustAcquire(t, dir)
	requireHeld(t, dir)
	if got := o.Dir(); got != dir {
		t.Fatalf("Dir() = %q, want %q", got, dir)
	}
	if err := o.Release(); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t, dir)
}

// A caller that drops the handle without releasing it must not lose the
// lock to the garbage collector closing the file.
func TestOwnerLockSurvivesDroppedHandle(t *testing.T) {
	dir := ownerTestDir(t)
	func() {
		if _, err := TryAcquireOwner(dir); err != nil {
			t.Fatalf("TryAcquireOwner: %v", err)
		}
	}()
	for range 5 {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	requireHeld(t, dir)
	t.Cleanup(func() {
		heldOwners.Range(func(k, _ any) bool {
			if o := k.(*Owner); o.Dir() == dir {
				_ = o.Release()
			}
			return true
		})
	})
}

func TestOwnerInfoFile(t *testing.T) {
	dir := ownerTestDir(t)
	o := mustAcquire(t, dir)
	path := filepath.Join(dir, OwnerInfoFile)

	check := func(want OwnerInfo) {
		t.Helper()
		want.PID = os.Getpid()
		got, err := ReadOwner(dir)
		if err != nil {
			t.Fatalf("ReadOwner: %v", err)
		}
		if got != want {
			t.Fatalf("ReadOwner = %+v, want %+v", got, want)
		}
		if runtime.GOOS != "windows" {
			st, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode := st.Mode().Perm(); mode != 0o600 {
				t.Fatalf("owner.json mode = %v, want 0600", mode)
			}
		}
		// Only the lock and the published file; no temporary files.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if strings.Join(names, ",") != OwnerInfoFile+","+OwnerLockFile {
			t.Fatalf("directory holds %v, want only %s and %s", names, OwnerInfoFile, OwnerLockFile)
		}
	}

	// The PID and host name are always this process's, whatever the
	// caller passes.
	in := helperNoAPI
	in.PID = 1
	in.Hostname = "elsewhere.example"
	if err := o.Publish(in); err != nil {
		t.Fatal(err)
	}
	check(helperNoAPI)

	if err := o.Publish(helperAPI); err != nil {
		t.Fatal(err)
	}
	check(helperAPI)

	ipv6 := helperAPI
	ipv6.Address = "[::1]:43210"
	if err := o.Publish(ipv6); err != nil {
		t.Fatalf("Publish with an IPv6 loopback address: %v", err)
	}
	check(ipv6)
	if err := o.Publish(helperAPI); err != nil {
		t.Fatal(err)
	}

	for name, edit := range map[string]func(*OwnerInfo){
		"address without token": func(i *OwnerInfo) { i.Token = "" },
		"token without address": func(i *OwnerInfo) { i.Address = "" },
		"no nebi version":       func(i *OwnerInfo) { i.NebiVersion = "" },
		"no client API version": func(i *OwnerInfo) { i.ClientAPIVersion = "" },
		"no kind":               func(i *OwnerInfo) { i.Kind = "" },
		"unknown kind":          func(i *OwnerInfo) { i.Kind = "daemon" },
		"private address":       func(i *OwnerInfo) { i.Address = "192.168.1.2:43210" },
		"unspecified address":   func(i *OwnerInfo) { i.Address = "0.0.0.0:43210" },
		"host name":             func(i *OwnerInfo) { i.Address = "localhost:43210" },
		"no port":               func(i *OwnerInfo) { i.Address = "127.0.0.1" },
		"port zero":             func(i *OwnerInfo) { i.Address = "127.0.0.1:0" },
		"port out of range":     func(i *OwnerInfo) { i.Address = "127.0.0.1:65536" },
	} {
		bad := helperAPI
		edit(&bad)
		if err := o.Publish(bad); err == nil {
			t.Fatalf("Publish accepted owner info with %s: %+v", name, bad)
		}
	}
	// A refused Publish leaves the published file alone.
	check(helperAPI)

	if err := o.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner.json after release: %v, want it removed", err)
	}
	if _, err := ReadOwner(dir); !errors.Is(err, ErrNoOwnerInfo) {
		t.Fatalf("ReadOwner after release: got %v, want ErrNoOwnerInfo", err)
	}
	// owner.lock stays: removing it would let two processes lock different
	// files of the same name.
	if _, err := os.Stat(filepath.Join(dir, OwnerLockFile)); err != nil {
		t.Fatalf("owner.lock after release: %v", err)
	}
	if err := o.Publish(helperNoAPI); !errors.Is(err, ErrOwnerReleased) {
		t.Fatalf("Publish after release: got %v, want ErrOwnerReleased", err)
	}
	if err := o.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

// A best-effort concurrency smoke test: a reader polls owner.json while the
// owner replaces it, and must only ever see one of the published values. It
// depends on timing, so it can miss a Publish that writes in place;
// TestOwnerInfoReplacedNotRewritten is the test that reliably catches that.
func TestOwnerInfoReplacedAtomically(t *testing.T) {
	dir := ownerTestDir(t)
	o := mustAcquire(t, dir)

	// Large values make a partly written file more likely to be seen.
	a := OwnerInfo{Kind: OwnerWeb, Hostname: thisHost, NebiVersion: "a", ClientAPIVersion: "v1", Address: "127.0.0.1:1", Token: strings.Repeat("a", 1<<20)}
	b := OwnerInfo{Kind: OwnerWeb, Hostname: thisHost, NebiVersion: "b", ClientAPIVersion: "v1", Address: "127.0.0.1:2", Token: strings.Repeat("b", 1<<20)}
	if err := o.Publish(a); err != nil {
		t.Fatal(err)
	}
	a.PID, b.PID = os.Getpid(), os.Getpid()

	stop := make(chan struct{})
	var (
		wg    sync.WaitGroup
		reads int
		bad   error
	)
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Windows cannot replace a file while it is open, so Publish
			// retries for a while. Leave it gaps between reads, as a real
			// reader polling now and then does.
			if runtime.GOOS == "windows" && reads > 0 {
				time.Sleep(2 * time.Millisecond)
			}
			got, err := ReadOwner(dir)
			reads++
			if err != nil {
				bad = fmt.Errorf("read %d: %w", reads, err)
				return
			}
			if got != a && got != b {
				bad = fmt.Errorf("read %d: content is neither published value (nebi_version %q, token length %d)",
					reads, got.NebiVersion, len(got.Token))
				return
			}
		}
	})

	for i := 0; i < 40; i++ {
		next := a
		if i%2 == 0 {
			next = b
		}
		if err := o.Publish(next); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	if bad != nil {
		t.Fatal(bad)
	}
	if reads == 0 {
		t.Fatal("reader never read owner.json")
	}
}

// Publish replaces owner.json with a new file rather than rewriting it, so
// a reader that opened the old one keeps reading it whole. Unlike the
// concurrent test above, this does not depend on timing.
func TestOwnerInfoReplacedNotRewritten(t *testing.T) {
	dir := ownerTestDir(t)
	o := mustAcquire(t, dir)
	path := filepath.Join(dir, OwnerInfoFile)

	if err := o.Publish(helperNoAPI); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	oldStat, err := old.Stat()
	if err != nil {
		t.Fatal(err)
	}
	// Windows cannot replace a file another handle has open (Publish waits
	// for it), so there the reader closes first and only identity is checked.
	keepOpen := runtime.GOOS != "windows"
	if !keepOpen {
		_ = old.Close()
	}

	if err := o.Publish(helperAPI); err != nil {
		t.Fatal(err)
	}
	newStat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(oldStat, newStat) {
		t.Fatal("Publish rewrote owner.json in place; readers can see it partly written")
	}
	if !keepOpen {
		return
	}
	data, err := io.ReadAll(old)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo := helperNoAPI
	wantInfo.PID = os.Getpid()
	want, _ := json.MarshalIndent(wantInfo, "", "  ")
	if string(data) != string(want)+"\n" {
		t.Fatalf("reader of the old owner.json saw %q, want the old content whole", data)
	}
}

func TestNewOwnerRemovesStaleOwnerInfo(t *testing.T) {
	dir := ownerTestDir(t)
	files := map[string]bool{ // name -> must survive
		OwnerInfoFile: false,
		ownerTempPrefix + "123" + ownerTempSuffix: false,
		"nebi-client.db":        true,
		"owner.json.bak":        true,
		ownerTempPrefix + "123": true,
	}
	for name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"pid":999999,"address":"127.0.0.1:9","token":"dead"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	mustAcquire(t, dir)
	for name, keep := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		switch {
		case keep && err != nil:
			t.Errorf("%s was removed: %v", name, err)
		case !keep && !errors.Is(err, os.ErrNotExist):
			t.Errorf("stale %s was not removed: %v", name, err)
		}
	}
	if _, err := ReadOwner(dir); !errors.Is(err, ErrNoOwnerInfo) {
		t.Fatalf("ReadOwner: got %v, want ErrNoOwnerInfo", err)
	}
}

func TestReadOwnerRejectsCorruptFile(t *testing.T) {
	dir := ownerTestDir(t)
	if err := os.WriteFile(filepath.Join(dir, OwnerInfoFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadOwner(dir)
	if err == nil || errors.Is(err, ErrNoOwnerInfo) {
		t.Fatalf("ReadOwner of a corrupt file: got %v, want a decode error", err)
	}
}

// A reader must never send the token to an address owner.json names unless
// it is loopback, whoever wrote the file. It still gets the PID and versions,
// so it can report a version skew.
func TestReadOwnerRejectsInvalidInfo(t *testing.T) {
	// kh is the kind and host name of a valid file, so each case below is
	// invalid only for the reason it names.
	host, _ := json.Marshal(thisHost)
	kh := `"kind":"web","hostname":` + string(host) + `,`
	web := OwnerInfo{Kind: OwnerWeb, Hostname: thisHost}
	with := func(info OwnerInfo) OwnerInfo {
		info.Kind, info.Hostname = web.Kind, web.Hostname
		return info
	}
	for name, tc := range map[string]struct {
		content string
		want    OwnerInfo
	}{
		"private address": {
			`{` + kh + `"pid":1,"nebi_version":"1","client_api_version":"v1","address":"10.0.0.1:80","token":"t"}`,
			with(OwnerInfo{PID: 1, NebiVersion: "1", ClientAPIVersion: "v1"}),
		},
		"host name": {
			`{` + kh + `"pid":1,"nebi_version":"1","client_api_version":"v1","address":"evil.example:80","token":"t"}`,
			with(OwnerInfo{PID: 1, NebiVersion: "1", ClientAPIVersion: "v1"}),
		},
		"no token": {
			`{` + kh + `"pid":1,"nebi_version":"1","client_api_version":"v1","address":"127.0.0.1:80"}`,
			with(OwnerInfo{PID: 1, NebiVersion: "1", ClientAPIVersion: "v1"}),
		},
		"no version": {
			`{` + kh + `"pid":1,"nebi_version":"1","address":"127.0.0.1:80","token":"t"}`,
			with(OwnerInfo{PID: 1, NebiVersion: "1"}),
		},
		"no pid": {
			`{` + kh + `"nebi_version":"1","client_api_version":"v1"}`,
			with(OwnerInfo{NebiVersion: "1", ClientAPIVersion: "v1"}),
		},
		"no kind": {
			`{"hostname":` + string(host) + `,"pid":1,"nebi_version":"1","client_api_version":"v1"}`,
			OwnerInfo{PID: 1, Hostname: thisHost, NebiVersion: "1", ClientAPIVersion: "v1"},
		},
		"unknown kind": {
			`{"kind":"daemon","hostname":` + string(host) + `,"pid":1,"nebi_version":"1","client_api_version":"v1"}`,
			OwnerInfo{PID: 1, Kind: "daemon", Hostname: thisHost, NebiVersion: "1", ClientAPIVersion: "v1"},
		},
		"no host name": {
			`{"kind":"web","pid":1,"nebi_version":"1","client_api_version":"v1","address":"127.0.0.1:80","token":"t"}`,
			OwnerInfo{PID: 1, Kind: OwnerWeb, NebiVersion: "1", ClientAPIVersion: "v1"},
		},
		// A newer owner with an address form this build does not know.
		"newer address form": {
			`{` + kh + `"pid":7,"nebi_version":"9.0.0","client_api_version":"v2","address":"unix:/run/nebi.sock","token":"t"}`,
			with(OwnerInfo{PID: 7, NebiVersion: "9.0.0", ClientAPIVersion: "v2"}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := ownerTestDir(t)
			if err := os.WriteFile(filepath.Join(dir, OwnerInfoFile), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			info, err := ReadOwner(dir)
			if !errors.Is(err, ErrInvalidOwnerInfo) {
				t.Fatalf("ReadOwner = %+v, %v; want ErrInvalidOwnerInfo", info, err)
			}
			if info.Address != "" || info.Token != "" {
				t.Fatalf("ReadOwner returned address %q, token set %t, with invalid owner info", info.Address, info.Token != "")
			}
			if info != tc.want {
				t.Fatalf("ReadOwner = %+v, want %+v", info, tc.want)
			}
		})
	}
}

// With a data directory shared between machines (a home directory on NFS),
// owner.json can come from another host. Its loopback address would reach
// this machine, so the reader fails and names the other host, and never
// gets the address or token.
func TestReadOwnerRejectsOtherHost(t *testing.T) {
	dir := ownerTestDir(t)
	o := mustAcquire(t, dir)

	orig := hostname
	t.Cleanup(func() { hostname = orig })
	hostname = func() (string, error) { return "build-box-7", nil }
	if err := o.Publish(helperAPI); err != nil {
		t.Fatal(err)
	}

	hostname = func() (string, error) { return "laptop", nil }
	info, err := ReadOwner(dir)
	if !errors.Is(err, ErrOwnerOnOtherHost) {
		t.Fatalf("ReadOwner = %+v, %v; want ErrOwnerOnOtherHost", info, err)
	}
	if !strings.Contains(err.Error(), `"build-box-7"`) {
		t.Errorf("error %q does not name the owner's host", err)
	}
	if info.Address != "" || info.Token != "" {
		t.Fatalf("ReadOwner returned address %q, token set %t, for an owner on another host", info.Address, info.Token != "")
	}
	if info.PID != os.Getpid() || info.Hostname != "build-box-7" || info.Kind != OwnerWeb {
		t.Errorf("ReadOwner = %+v, want the PID, kind and host name of the owner", info)
	}

	// Host names compare without case, as DNS names do.
	hostname = func() (string, error) { return "BUILD-BOX-7", nil }
	if info, err := ReadOwner(dir); err != nil || info.Address != helperAPI.Address {
		t.Fatalf("ReadOwner on the same host in other case = %+v, %v", info, err)
	}

	// A host name that cannot be found fails Publish and ReadOwner rather
	// than skip the check.
	hostname = func() (string, error) { return "", errors.New("no name") }
	if err := o.Publish(helperAPI); err == nil {
		t.Error("Publish succeeded without a host name")
	}
	if _, err := ReadOwner(dir); err == nil {
		t.Error("ReadOwner succeeded without a host name")
	}
}

// Every owner publishes its kind, so a waiting process can tell an app that
// is still starting from a CLI command.
func TestOwnerKind(t *testing.T) {
	for kind, serves := range map[OwnerKind]bool{OwnerDesktop: true, OwnerWeb: true, OwnerCLI: false} {
		if kind.ServesAPI() != serves {
			t.Errorf("%s.ServesAPI() = %t, want %t", kind, !serves, serves)
		}
		dir := ownerTestDir(t)
		o := mustAcquire(t, dir)
		info := helperNoAPI
		info.Kind = kind
		// Publish writes this machine's host name, whatever the caller set.
		info.Hostname = "not-" + thisHost
		if err := o.Publish(info); err != nil {
			t.Fatalf("Publish with kind %s: %v", kind, err)
		}
		got, err := ReadOwner(dir)
		if err != nil || got.Kind != kind || got.Hostname != thisHost {
			t.Fatalf("ReadOwner = %+v, %v; want kind %s on host %s", got, err, kind, thisHost)
		}
		data, err := os.ReadFile(filepath.Join(dir, OwnerInfoFile))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"kind": "`+string(kind)+`"`) ||
			!strings.Contains(string(data), `"hostname": `) {
			t.Errorf("owner.json %s lacks the kind or host name", data)
		}
	}
}

// Printing or logging OwnerInfo must not reveal the token.
func TestOwnerInfoRedactsToken(t *testing.T) {
	info := helperAPI
	info.PID = 42
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(verb, info)
		if strings.Contains(out, info.Token) {
			t.Errorf("%s printed the token: %s", verb, out)
		}
		if !strings.Contains(out, info.Address) || !strings.Contains(out, redactedToken) {
			t.Errorf("%s = %s, want the address and a redacted token", verb, out)
		}
	}

	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("owner", "owner", info)
	if strings.Contains(buf.String(), info.Token) {
		t.Errorf("slog printed the token: %s", buf.String())
	}
	for _, field := range []string{`"client_api_version":"v1"`, `"kind":"web"`, `"hostname":`} {
		if !strings.Contains(buf.String(), field) {
			t.Errorf("slog output %s lacks the owner's field %s", buf.String(), field)
		}
	}

	// An unset token prints as empty, not as redacted.
	if out := fmt.Sprint(helperNoAPI); strings.Contains(out, redactedToken) {
		t.Errorf("owner info without a token printed %s", out)
	}
	// The JSON in owner.json still carries the token.
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), info.Token) {
		t.Errorf("JSON lost the token: %s", data)
	}
}

// The owner lock sits below every client entry point, so it must not depend
// on any of them, directly or through another nebi package.
func TestOwnerLockImports(t *testing.T) {
	root, module := moduleRoot(t)
	forbidden := func(pkg string) bool {
		rel, ok := strings.CutPrefix(pkg, module)
		if !ok {
			return false
		}
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || strings.HasPrefix(rel, "cmd/") {
			return true // the desktop app (package main at the root) and the binaries
		}
		for _, seg := range strings.Split(rel, "/") {
			switch seg {
			case "api", "cli", "web", "desktop":
				return true
			}
		}
		return false
	}

	self := module + "/internal/client/jobs"
	seen := map[string]bool{}
	var walk func(pkg string, chain []string)
	walk = func(pkg string, chain []string) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		chain = append(chain, pkg)
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(pkg, module), "/")))
		for _, imp := range packageImports(t, dir) {
			if !strings.HasPrefix(imp, module) {
				continue
			}
			if forbidden(imp) {
				t.Errorf("forbidden import: %s -> %s", strings.Join(chain, " -> "), imp)
				continue
			}
			walk(imp, chain)
		}
	}
	walk(self, nil)
}

// packageImports returns the imports of the non-test Go files in dir, for
// every platform.
func packageImports(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var imports []string
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			imports = append(imports, path)
		}
	}
	return imports
}

func moduleRoot(t *testing.T) (dir, module string) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if m, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					return dir, strings.TrimSpace(m)
				}
			}
			t.Fatalf("no module line in %s/go.mod", dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
