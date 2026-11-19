package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
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
	helperNoAPI = OwnerInfo{NebiVersion: "1.2.3", ClientAPIVersion: "v1"}
	helperAPI   = OwnerInfo{NebiVersion: "1.2.3", ClientAPIVersion: "v1",
		Address: "127.0.0.1:43210", Token: "helper-token"}
)

// TestOwnerHelperProcess is the second process of the owner lock tests. It
// is not a test: without ownerHelperEnv it returns at once.
//
// Modes:
//   - try: TryAcquireOwner, print "acquired" or "held". The owner then obeys
//     stdin: "publish" publishes helperNoAPI, "publish-api" publishes
//     helperAPI, and EOF releases the lock and prints "released".
//   - wait: like try, but takes the lock with AcquireOwner.
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
	if mode == "wait" {
		o, err = AcquireOwner(context.Background(), dir)
	} else {
		o, err = TryAcquireOwner(dir)
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
	// The very next attempt succeeds, with no cleanup step in between, and
	// the new owner has removed the dead owner's details.
	o := mustAcquire(t, dir)
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
		_, _ = child.Wait() // reaped by init on Unix; harmless error there
	})
	requireHeld(t, dir)

	h.kill()

	mustAcquire(t, dir)
	// The lock was free while the child still ran. (Signal 0 only probes
	// whether a process exists; Windows has no equivalent.)
	if runtime.GOOS != "windows" {
		if err := child.Signal(syscall.Signal(0)); err != nil {
			t.Fatalf("child %d of the killed owner is not running, so the test proved nothing: %v", pid, err)
		}
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
	// The helper is waiting; give it time to try at least once.
	time.Sleep(10 * lockPollInterval)
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

	// The PID is always this process's, whatever the caller passes.
	in := helperNoAPI
	in.PID = 1
	if err := o.Publish(in); err != nil {
		t.Fatal(err)
	}
	check(helperNoAPI)

	if err := o.Publish(helperAPI); err != nil {
		t.Fatal(err)
	}
	check(helperAPI)

	for _, bad := range []OwnerInfo{
		{Address: "127.0.0.1:1"},
		{Token: "secret"},
	} {
		if err := o.Publish(bad); err == nil {
			t.Fatalf("Publish(%+v) accepted an address without a token or a token without an address", bad)
		}
	}
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

func TestOwnerInfoReplacedAtomically(t *testing.T) {
	dir := ownerTestDir(t)
	o := mustAcquire(t, dir)

	// Big enough that a write is not a single small syscall, so a reader of
	// a file written in place would see it empty or cut short.
	a := OwnerInfo{NebiVersion: "a", ClientAPIVersion: "v1", Address: "127.0.0.1:1", Token: strings.Repeat("a", 1<<20)}
	b := OwnerInfo{NebiVersion: "b", ClientAPIVersion: "v1", Address: "127.0.0.1:2", Token: strings.Repeat("b", 1<<20)}
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
	want, _ := json.MarshalIndent(OwnerInfo{PID: os.Getpid(), NebiVersion: helperNoAPI.NebiVersion,
		ClientAPIVersion: helperNoAPI.ClientAPIVersion}, "", "  ")
	if string(data) != string(want)+"\n" {
		t.Fatalf("reader of the old owner.json saw %q, want the old content whole", data)
	}
}

func TestNewOwnerRemovesStaleOwnerInfo(t *testing.T) {
	dir := ownerTestDir(t)
	files := map[string]bool{ // name -> must survive
		OwnerInfoFile: false,
		ownerTempPrefix + "123" + ownerTempSuffix: false,
		"nebi.db":               true,
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
