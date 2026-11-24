package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
)

// The cancel tests need real process trees. They run this test binary again
// with cancelHelperEnv set; TestCancelHelperProcess then plays one process of
// the tree instead of running as a test.
const (
	cancelHelperEnv    = "NEBI_TEST_PROCESS_HELPER"
	cancelHelperDirEnv = "NEBI_TEST_PROCESS_DIR"

	// detachRefusedFile is the file a detacher writes, in place of its PID,
	// when the system refuses to start its child outside the command's
	// process tree.
	detachRefusedFile = "detach-refused"

	// cancelTimeout bounds every wait in these tests. It is far above what
	// any step needs, so it only trips when a step is really stuck.
	cancelTimeout = 20 * time.Second
)

// TestCancelHelperProcess is one process of a cancel test's tree. It is not
// a test: without cancelHelperEnv it returns at once.
//
// Every mode prints "<mode> started" to stdout. Every mode except
// spawn-exit, print and fail also records its PID as a file in the
// directory named by cancelHelperDirEnv. Children inherit stdout and
// stderr, so they hold the command's output pipe as long as they run.
//
// Modes:
//   - sleep: block until killed.
//   - parent: start a sleep child, then wait for it.
//   - grandparent: start a parent child, then wait for it.
//   - orphaner: start a spawn-exit child and wait for it, so the sleep
//     process it started is left without a running parent; then block
//     until killed.
//   - spawn-exit: start a sleep child and exit without waiting for it.
//   - detacher: start a sleep child outside the command's process tree
//     (see detach), then block until killed. If the system refuses, write
//     the reason to detachRefusedFile and exit.
//   - owner: start a parent tree through CommandContext, as nebi starts
//     pixi, then block until killed.
//   - print: print "print stderr" to stderr and exit 0.
//   - fail: print "fail stderr" to stderr and exit 1.
func TestCancelHelperProcess(t *testing.T) {
	mode := os.Getenv(cancelHelperEnv)
	if mode == "" {
		return
	}
	dir := os.Getenv(cancelHelperDirEnv)
	fail := func(err error) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", mode, err)
		os.Exit(2)
	}
	start := func(childMode string, prepare func(*exec.Cmd)) *exec.Cmd {
		child := helperCommand(childMode, dir)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if prepare != nil {
			prepare(child)
		}
		if err := child.Start(); err != nil {
			fail(err)
		}
		return child
	}

	fmt.Fprintf(os.Stdout, "%s started\n", mode)
	var child *exec.Cmd
	switch mode {
	case "sleep":
	case "parent":
		child = start("sleep", nil)
	case "grandparent":
		child = start("parent", nil)
	case "orphaner":
		if err := start("spawn-exit", nil).Wait(); err != nil {
			fail(err)
		}
	case "spawn-exit":
		start("sleep", nil)
		os.Exit(0)
	case "detacher":
		d := helperCommand("sleep", dir)
		d.Stdout, d.Stderr = os.Stdout, os.Stderr
		detach(d)
		if err := d.Start(); err != nil {
			if werr := os.WriteFile(filepath.Join(dir, detachRefusedFile), []byte(err.Error()), 0o600); werr != nil {
				fail(werr)
			}
			os.Exit(4)
		}
	case "print", "fail":
		fmt.Fprintf(os.Stderr, "%s stderr\n", mode)
		if mode == "fail" {
			os.Exit(1)
		}
		os.Exit(0)
	case "owner":
		helper := helperCommand("parent", dir)
		cmd := CommandContext(context.Background(), helper.Path, helper.Args[1:], limits.ProcessLimits{})
		cmd.Env = helper.Env
		if err := cmd.Start(); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("unknown helper mode %q", mode))
	}
	if err := recordPID(dir, mode); err != nil {
		fail(err)
	}
	if child != nil {
		_ = child.Wait()
		os.Exit(0)
	}
	time.Sleep(10 * time.Minute) // until killed
	os.Exit(3)
}

func helperCommand(mode, dir string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCancelHelperProcess$")
	cmd.Env = append(os.Environ(), cancelHelperEnv+"="+mode, cancelHelperDirEnv+"="+dir)
	return cmd
}

// recordPID writes this process's PID into dir. The file appears complete,
// so a reader never sees a partial one.
func recordPID(dir, mode string) error {
	name := filepath.Join(dir, strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(name+".tmp", []byte(mode), 0o600); err != nil {
		return err
	}
	return os.Rename(name+".tmp", name)
}

// TestCancelStopsProcessTree cancels a command whose process started a child
// that started a grandchild, all holding the command's output pipe. Cancel
// must stop all three, and Wait must return because they are gone, not
// because WaitDelay gave up on the pipe.
func TestCancelStopsProcessTree(t *testing.T) {
	dir := t.TempDir()
	cmd, out, cancel := startHelper(t, "grandparent", dir)
	pids := waitForPIDs(t, dir, 3)

	cancel()
	assertStoppedPromptly(t, cmd, pids)
	for _, mode := range []string{"grandparent", "parent", "sleep"} {
		if !strings.Contains(out.String(), mode+" started") {
			t.Fatalf("output of %s missing from %q", mode, out.String())
		}
	}
}

// TestCancelStopsOrphanedDescendants cancels a command whose process left a
// grandchild running after the grandchild's parent exited. Nothing links
// that grandchild to the command's process any more, but it was started
// inside the command's process group or job, so cancel must stop it too.
func TestCancelStopsOrphanedDescendants(t *testing.T) {
	dir := t.TempDir()
	cmd, out, cancel := startHelper(t, "orphaner", dir)
	pids := waitForPIDs(t, dir, 2)

	cancel()
	assertStoppedPromptly(t, cmd, pids)
	if !strings.Contains(out.String(), "sleep started") {
		t.Fatalf("the orphan must hold the output pipe, got %q", out.String())
	}
}

// TestCancelWaitReturnsWhenOutputHeldOutsideTree cancels a command whose
// process started a child outside the command's process tree: in a process
// group of its own on unix, or broken away from the job on Windows. Cancel
// cannot reach that child, which keeps running and holds the output pipe;
// Wait must still return, once WaitDelay gives up on the pipe.
func TestCancelWaitReturnsWhenOutputHeldOutsideTree(t *testing.T) {
	dir := t.TempDir()
	cmd, out, cancel := startHelper(t, "detacher", dir)
	pids := waitForDetacher(t, dir)

	cancel()
	elapsed, err := waitBounded(t, cmd)
	if err == nil {
		t.Fatal("a cancelled command must fail")
	}
	if limit := cmd.WaitDelay + 5*time.Second; elapsed > limit {
		t.Fatalf("Wait took %v, more than %v", elapsed, limit)
	}
	for pid, mode := range pids {
		if mode == "detacher" {
			waitProcessGone(t, pid, mode)
		} else if processGone(pid) {
			t.Fatalf("the detached %s process %d was stopped by the cancel; it was not outside the tree", mode, pid)
		}
	}
	if !strings.Contains(out.String(), "sleep started") {
		t.Fatalf("the detached child must hold the output pipe, got %q", out.String())
	}
}

// waitForDetacher waits until a detacher and its detached child have
// recorded their PIDs, and returns them. It skips the test if the system
// refused to start the child outside the tree: on Windows, a job the test
// itself runs in may not allow breakaway.
func waitForDetacher(t *testing.T, dir string) map[int]string {
	t.Helper()
	deadline := time.Now().Add(cancelTimeout)
	for {
		if reason, err := os.ReadFile(filepath.Join(dir, detachRefusedFile)); err == nil {
			t.Skipf("this system does not let a process start a child outside its tree: %s", reason)
		}
		pids, err := readPIDs(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(pids) == 2 {
			return pids
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of 2 processes started: %v", len(pids), pids)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestExitLeavesChildrenRunning runs a command that starts a child and exits
// on its own. Only cancelling stops the processes a command started: like a
// process group on unix, the job on Windows must not take the child down
// when the command ends.
func TestExitLeavesChildrenRunning(t *testing.T) {
	dir := t.TempDir()
	helper := helperCommand("spawn-exit", dir)
	cmd := CommandContext(context.Background(), helper.Path, helper.Args[1:], limits.ProcessLimits{})
	cmd.Env = helper.Env
	t.Cleanup(func() { killRecorded(dir) })
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	pids := waitForPIDs(t, dir, 1)

	// Give a kill that is under way time to land before looking.
	time.Sleep(500 * time.Millisecond)
	for pid, mode := range pids {
		if processGone(pid) {
			t.Fatalf("%s process %d was stopped when the command exited", mode, pid)
		}
	}
}

// startHelper starts the helper in mode through CommandContext, with its
// output going through a pipe the way the job runner's log does. It kills
// every recorded process when the test ends, whatever the test found.
func startHelper(t *testing.T, mode, dir string) (*Cmd, *syncBuffer, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	helper := helperCommand(mode, dir)
	cmd := CommandContext(ctx, helper.Path, helper.Args[1:], limits.ProcessLimits{})
	cmd.Env = helper.Env
	out := new(syncBuffer)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { killRecorded(dir) })
	return cmd, out, cancel
}

// assertStoppedPromptly waits for a cancelled cmd and checks that every
// process in pids is gone. Wait must return before WaitDelay: returning only
// once WaitDelay gave up on the output pipe means a process still held it.
func assertStoppedPromptly(t *testing.T, cmd *Cmd, pids map[int]string) {
	t.Helper()
	elapsed, err := waitBounded(t, cmd)
	if err == nil {
		t.Fatal("a cancelled command must fail")
	}
	if elapsed >= cmd.WaitDelay {
		t.Fatalf("Wait took %v; the tree was not stopped before WaitDelay (%v)", elapsed, cmd.WaitDelay)
	}
	for pid, mode := range pids {
		waitProcessGone(t, pid, mode)
	}
}

// waitBounded waits for cmd, failing the test if Wait does not return.
func waitBounded(t *testing.T, cmd *Cmd) (time.Duration, error) {
	t.Helper()
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return time.Since(start), err
	case <-time.After(cancelTimeout):
		t.Fatal("Wait did not return after cancel")
		return 0, nil
	}
}

// waitForPIDs waits until n helper processes have recorded their PIDs, and
// returns them with their modes.
func waitForPIDs(t *testing.T, dir string, n int) map[int]string {
	t.Helper()
	deadline := time.Now().Add(cancelTimeout)
	for {
		pids, err := readPIDs(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(pids) == n {
			return pids
		}
		if len(pids) > n {
			t.Fatalf("want %d processes, got %v", n, pids)
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d processes started: %v", len(pids), n, pids)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readPIDs(dir string) (map[int]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	pids := map[int]string{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // a .tmp file not yet renamed
		}
		mode, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		pids[pid] = string(mode)
	}
	return pids, nil
}

// waitProcessGone waits until pid no longer runs.
func waitProcessGone(t *testing.T, pid int, mode string) {
	t.Helper()
	deadline := time.Now().Add(cancelTimeout)
	for !processGone(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("%s process %d still runs", mode, pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// killRecorded kills every recorded helper process still running, so a
// failed test does not leave processes behind.
func killRecorded(dir string) {
	pids, _ := readPIDs(dir)
	for pid := range pids {
		if processGone(pid) {
			continue
		}
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
			_ = p.Release()
		}
	}
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes of a command's
// stdout and stderr copiers and the test's reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
